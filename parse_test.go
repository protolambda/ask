package ask

import (
	"errors"
	"flag"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// testFlag creates a flag with the given path, optional shorthand, and either a bool (implicit) or string value.
func testFlag(path string, shorthand byte, implicit bool) PrefixedFlag {
	var v flag.Value
	if implicit {
		v = new(BoolValue)
	} else {
		v = new(StringValue)
	}
	name := path
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		name = path[i+1:]
	}
	return PrefixedFlag{Path: path, Flag: &Flag{Value: v, Name: name, Shorthand: shorthand}}
}

// sortTestFlags splits flags into the sorted short and long lists that the parser expects,
// like cmdDescription.applyArgs does.
func sortTestFlags(flags ...PrefixedFlag) (short, long []PrefixedFlag) {
	for _, f := range flags {
		if f.Shorthand != 0 {
			short = append(short, f)
		}
		if string(f.Shorthand) != f.Name {
			long = append(long, f)
		}
	}
	sort.SliceStable(short, func(i, j int) bool { return short[i].Shorthand < short[j].Shorthand })
	sort.SliceStable(long, func(i, j int) bool { return long[i].Path < long[j].Path })
	return
}

var errBadTestValue = errors.New("bad test value")

// recordArgs returns an ApplyArg that records "path=value" entries,
// and fails with errBadTestValue if the value is "bad".
func recordArgs(applied *[]string) ApplyArg {
	return func(fl PrefixedFlag, value string) error {
		if value == "bad" {
			return errBadTestValue
		}
		*applied = append(*applied, fl.Path+"="+value)
		return nil
	}
}

func TestParseArgs(t *testing.T) {
	short, long := sortTestFlags(
		testFlag("verbose", 'v', true),
		testFlag("port", 'p', false),
		testFlag("name", 0, false),
		testFlag("x", 'x', false), // shorthand-only
		testFlag("grp.flag", 0, false),
		// shorthands at the boundaries of the sorted shorthand list
		testFlag("aa", 'a', true),
		testFlag("mm", 'm', true),
		testFlag("zz", 'z', true),
		// sorts right after "help", must not be confused with it
		testFlag("help-me", 0, true),
	)

	cases := []struct {
		name          string
		args          []string
		wantApplied   []string
		wantRemaining []string
		// wantErr is a substring of the expected error, empty if no error is expected
		wantErr string
		// wantErrIs, if not nil, must be in the chain of the returned error
		wantErrIs error
	}{
		// long flags
		{name: "long with equals", args: []string{"--port=80"}, wantApplied: []string{"port=80"}},
		{name: "long with separate value", args: []string{"--port", "80"}, wantApplied: []string{"port=80"}},
		{name: "long empty value", args: []string{"--port="}, wantApplied: []string{"port="}},
		{name: "long value with equals", args: []string{"--port=a=b"}, wantApplied: []string{"port=a=b"}},
		{name: "long negative value", args: []string{"--port", "-1"}, wantApplied: []string{"port=-1"}},
		{name: "long bool implicit", args: []string{"--verbose"}, wantApplied: []string{"verbose=true"}},
		{name: "long bool explicit", args: []string{"--verbose=false"}, wantApplied: []string{"verbose=false"}},
		{name: "long bool does not consume next arg", args: []string{"--verbose", "pos"},
			wantApplied: []string{"verbose=true"}, wantRemaining: []string{"pos"}},
		{name: "long dotted path", args: []string{"--grp.flag=1"}, wantApplied: []string{"grp.flag=1"}},
		{name: "long multiple", args: []string{"--name", "n", "--port=1", "--verbose"},
			wantApplied: []string{"name=n", "port=1", "verbose=true"}},
		{name: "long unrecognized", args: []string{"--nope"}, wantErr: "unrecognized flag: nope"},
		{name: "long unrecognized prefix of known", args: []string{"--por"}, wantErr: "unrecognized flag: por"},
		{name: "long unrecognized extension of known", args: []string{"--ports"}, wantErr: "unrecognized flag: ports"},
		{name: "long missing value", args: []string{"--port"}, wantErr: "flag needs an argument: --port"},
		{name: "long bad syntax triple dash", args: []string{"---port"}, wantErr: "bad flag syntax"},
		{name: "long bad syntax equals", args: []string{"--=x"}, wantErr: "bad flag syntax"},
		{name: "long apply error", args: []string{"--port", "bad"}, wantErr: `failed to apply flag port: "bad"`, wantErrIs: errBadTestValue},
		{name: "long help", args: []string{"--help"}, wantErrIs: HelpErr},
		{name: "long help with value", args: []string{"--help=x"}, wantErrIs: HelpErr},
		{name: "long help after flags", args: []string{"--port=1", "--help"}, wantApplied: []string{"port=1"}, wantErrIs: HelpErr},
		{name: "long help-me is a regular flag", args: []string{"--help-me"}, wantApplied: []string{"help-me=true"}},

		// short flags
		{name: "short implicit", args: []string{"-v"}, wantApplied: []string{"verbose=true"}},
		{name: "short combined implicit", args: []string{"-vam"}, wantApplied: []string{"verbose=true", "aa=true", "mm=true"}},
		{name: "short repeated", args: []string{"-vv"}, wantApplied: []string{"verbose=true", "verbose=true"}},
		{name: "short first in sorted list", args: []string{"-a"}, wantApplied: []string{"aa=true"}},
		{name: "short last in sorted list", args: []string{"-z"}, wantApplied: []string{"zz=true"}},
		{name: "short separate value", args: []string{"-p", "80"}, wantApplied: []string{"port=80"}},
		{name: "short attached value", args: []string{"-p80"}, wantApplied: []string{"port=80"}},
		{name: "short equals value", args: []string{"-p=80"}, wantApplied: []string{"port=80"}},
		{name: "short equals empty value", args: []string{"-p="}, wantApplied: []string{"port="}},
		{name: "short negative value", args: []string{"-p", "-1"}, wantApplied: []string{"port=-1"}},
		{name: "short combined then separate value", args: []string{"-vp", "80"}, wantApplied: []string{"verbose=true", "port=80"}},
		{name: "short combined then attached value", args: []string{"-vp80"}, wantApplied: []string{"verbose=true", "port=80"}},
		{name: "short combined then equals value", args: []string{"-vp=80"}, wantApplied: []string{"verbose=true", "port=80"}},
		{name: "short non-implicit takes rest as value", args: []string{"-pv"}, wantApplied: []string{"port=v"}},
		{name: "shorthand-only flag", args: []string{"-x", "1"}, wantApplied: []string{"x=1"}},
		{name: "shorthand-only flag has no long form", args: []string{"--x", "1"}, wantErr: "unrecognized flag: x"},
		{name: "short multiple consuming values", args: []string{"-p", "1", "-x", "2", "pos"},
			wantApplied: []string{"port=1", "x=2"}, wantRemaining: []string{"pos"}},
		{name: "short bool does not consume next arg", args: []string{"-v", "pos"},
			wantApplied: []string{"verbose=true"}, wantRemaining: []string{"pos"}},
		{name: "short unknown", args: []string{"-b"}, wantErr: "unknown shorthand flag: 'b' in -b"},
		{name: "short unknown between known", args: []string{"-n"}, wantErr: "unknown shorthand flag: 'n' in -n"},
		{name: "short unknown after known", args: []string{"-vb"}, wantApplied: []string{"verbose=true"}, wantErr: "unknown shorthand flag: 'b' in -b"},
		{name: "short missing value", args: []string{"-p"}, wantErr: "flag needs an argument: 'p' in -p"},
		{name: "short apply error", args: []string{"-p", "bad"}, wantErr: `failed to apply flag p: "bad"`, wantErrIs: errBadTestValue},
		{name: "short apply error attached", args: []string{"-pbad"}, wantErr: `failed to apply flag p: "bad"`, wantErrIs: errBadTestValue},
		{name: "short help", args: []string{"-h"}, wantErrIs: HelpErr},
		{name: "short help after combined", args: []string{"-vh"}, wantApplied: []string{"verbose=true"}, wantErrIs: HelpErr},

		// non-flag args
		{name: "no args", args: nil},
		{name: "stop at positional", args: []string{"pos", "--port", "1"}, wantRemaining: []string{"pos", "--port", "1"}},
		{name: "single dash is positional", args: []string{"-", "--port", "1"}, wantRemaining: []string{"-", "--port", "1"}},
		{name: "empty arg is positional", args: []string{"", "-v"}, wantRemaining: []string{"", "-v"}},
		{name: "terminator", args: []string{"--port", "1", "--", "--port", "2", "-v"},
			wantApplied: []string{"port=1"}, wantRemaining: []string{"--port", "2", "-v"}},
		{name: "terminator without rest", args: []string{"-v", "--"}, wantApplied: []string{"verbose=true"}},
		{name: "terminator as value", args: []string{"--port", "--"}, wantApplied: []string{"port=--"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var applied []string
			remaining, err := ParseArgs(short, long, tc.args, recordArgs(&applied))
			if tc.wantErr == "" && tc.wantErrIs == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing %q, got none (applied %v, remaining %v)", tc.wantErr, applied, remaining)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
				}
				if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("expected error chain to contain %v, got: %v", tc.wantErrIs, err)
				}
			}
			if got, want := fmt.Sprint(applied), fmt.Sprint(tc.wantApplied); got != want {
				t.Errorf("applied flags: got %s, want %s", got, want)
			}
			if err == nil {
				if got, want := fmt.Sprint(remaining), fmt.Sprint(tc.wantRemaining); got != want {
					t.Errorf("remaining args: got %s, want %s", got, want)
				}
			}
		})
	}
}

func TestParseArgs_noFlags(t *testing.T) {
	var applied []string
	if _, err := ParseArgs(nil, nil, []string{"-v"}, recordArgs(&applied)); err == nil || !strings.Contains(err.Error(), "unknown shorthand flag") {
		t.Fatalf("expected unknown shorthand error, got: %v", err)
	}
	if _, err := ParseArgs(nil, nil, []string{"--verbose"}, recordArgs(&applied)); err == nil || !strings.Contains(err.Error(), "unrecognized flag") {
		t.Fatalf("expected unrecognized flag error, got: %v", err)
	}
	if _, err := ParseArgs(nil, nil, []string{"-h"}, recordArgs(&applied)); !errors.Is(err, HelpErr) {
		t.Fatalf("expected help error, got: %v", err)
	}
	if _, err := ParseArgs(nil, nil, []string{"--help"}, recordArgs(&applied)); !errors.Is(err, HelpErr) {
		t.Fatalf("expected help error, got: %v", err)
	}
	remaining, err := ParseArgs(nil, nil, []string{"a", "b"}, recordArgs(&applied))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := fmt.Sprint(remaining); got != "[a b]" {
		t.Fatalf("unexpected remaining args: %s", got)
	}
}

// A user-defined help flag takes precedence over the built-in help handling.
func TestParseArgs_userDefinedHelp(t *testing.T) {
	short, long := sortTestFlags(testFlag("help", 'h', true))
	for _, args := range [][]string{{"--help"}, {"-h"}} {
		var applied []string
		if _, err := ParseArgs(short, long, args, recordArgs(&applied)); err != nil {
			t.Fatalf("%v: unexpected error: %v", args, err)
		}
		if got := fmt.Sprint(applied); got != "[help=true]" {
			t.Fatalf("%v: expected help flag to be applied, got: %s", args, got)
		}
	}
}

func TestParseShortArg_empty(t *testing.T) {
	if _, err := ParseShortArg(nil, "", nil, nil); err == nil {
		t.Fatal("expected error for empty arg")
	}
	if _, _, err := parseSingleShortArg(nil, "", nil, nil); err == nil {
		t.Fatal("expected error for empty shorthands")
	}
}

func TestParseLongArg_tooShort(t *testing.T) {
	if _, err := ParseLongArg(nil, "-", nil, nil); err == nil {
		t.Fatal("expected error for too short arg")
	}
	if _, err := ParseLongArg(nil, "--", nil, nil); err == nil {
		t.Fatal("expected error for empty name")
	}
}
