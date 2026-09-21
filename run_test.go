package ask

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

// defFlag is a custom flag value with its own default.
type defFlag struct {
	v     string
	calls int
}

func (f *defFlag) Default() {
	f.calls++
	f.v = "flag-default"
}

func (f *defFlag) Set(s string) error {
	f.v = s
	return nil
}

func (f *defFlag) String() string {
	return f.v
}

// defGroup is a flag group with its own default, which overrides the default of its flag value.
type defGroup struct {
	Calls int
	Val   string  `ask:"--val"`
	Fl    defFlag `ask:"--fl"`
}

func (g *defGroup) Default() {
	g.Calls++
	g.Val = "group-default"
	g.Fl.v = "group-overrides-flag"
}

// defCmd is a command with its own default, which overrides the default of an embedded group,
// and replaces a pointer group entirely.
type defCmd struct {
	Calls  int
	Inline defGroup  `ask:"."`
	Sub    defGroup  `ask:".sub"`
	Ptr    *defGroup `ask:".ptr"`
	Top    string    `ask:"--top"`
	ran    bool
}

func (c *defCmd) Default() {
	c.Calls++
	c.Top = "cmd-default"
	c.Sub.Val = "cmd-overrides-group"
	c.Ptr = &defGroup{Val: "cmd-replaced-ptr"}
}

func (c *defCmd) Run(ctx context.Context) error {
	c.ran = true
	return nil
}

func TestRun_defaults(t *testing.T) {
	cmd := new(defCmd)
	if err := Run(context.Background(), cmd, []string{"--ptr.val=x", "--sub.fl=y"}); err != nil {
		t.Fatal(err)
	}
	if !cmd.ran {
		t.Fatal("expected command to run")
	}
	// each Default runs exactly once
	if cmd.Calls != 1 || cmd.Inline.Calls != 1 || cmd.Sub.Calls != 1 {
		t.Fatalf("expected each default to run once, got cmd: %d, inline: %d, sub: %d", cmd.Calls, cmd.Inline.Calls, cmd.Sub.Calls)
	}
	if cmd.Inline.Fl.calls != 1 || cmd.Sub.Fl.calls != 1 {
		t.Fatalf("expected each flag default to run once, got inline: %d, sub: %d", cmd.Inline.Fl.calls, cmd.Sub.Fl.calls)
	}
	// defaults apply bottom-up, the embedding struct has the final say
	if cmd.Top != "cmd-default" {
		t.Fatalf("expected command default, got %q", cmd.Top)
	}
	if cmd.Inline.Val != "group-default" {
		t.Fatalf("expected group default, got %q", cmd.Inline.Val)
	}
	if cmd.Inline.Fl.v != "group-overrides-flag" {
		t.Fatalf("expected group to override flag default, got %q", cmd.Inline.Fl.v)
	}
	if cmd.Sub.Val != "cmd-overrides-group" {
		t.Fatalf("expected command to override group default, got %q", cmd.Sub.Val)
	}
	// user input overrides all defaults
	if cmd.Sub.Fl.v != "y" {
		t.Fatalf("expected flag to be set, got %q", cmd.Sub.Fl.v)
	}
	// a pointer group replaced by the command default is what the flags are bound to,
	// its own Default is not applied again.
	if cmd.Ptr.Val != "x" {
		t.Fatalf("expected flag to be set on replaced pointer group, got %q", cmd.Ptr.Val)
	}
	if cmd.Ptr.Calls != 0 {
		t.Fatalf("expected no default call on replaced pointer group, got %d", cmd.Ptr.Calls)
	}

	// The usage shows the defaults as they are after all defaults were applied
	err := Run(context.Background(), new(defCmd), []string{"--help"})
	if !errors.Is(err, HelpErr) {
		t.Fatalf("expected help error, got %v", err)
	}
	usage := UsageFromErr(err)
	for flagName, wantDefault := range map[string]string{
		"--top":     "(default: cmd-default)",
		"--val":     "(default: group-default)",
		"--fl":      "(default: group-overrides-flag)",
		"--sub.val": "(default: cmd-overrides-group)",
		"--sub.fl":  "(default: group-overrides-flag)",
		"--ptr.val": "(default: cmd-replaced-ptr)",
		// the group replaced by the command default had no defaults applied
		"--ptr.fl": "(env: PTR_FL)",
	} {
		line := usageLine(usage, flagName)
		if line == "" {
			t.Errorf("expected usage to contain flag %q, got:\n%s", flagName, usage)
			continue
		}
		if !strings.Contains(line, wantDefault) {
			t.Errorf("expected usage line of %q to contain %q, got: %q", flagName, wantDefault, line)
		}
		if wantDefault[:5] != "(defa" && strings.Contains(line, "(default:") {
			t.Errorf("expected no default in usage line of %q, got: %q", flagName, line)
		}
	}
}

// usageLine returns the usage line of the given flag, or an empty string if not found.
func usageLine(usage string, flagName string) string {
	for _, line := range strings.Split(usage, "\n") {
		if strings.HasPrefix(line, "  "+flagName+" ") {
			return line
		}
	}
	return ""
}

type shortCmd struct {
	Verbose bool   `ask:"--verbose -v" help:"verbose output"`
	Port    uint16 `ask:"--port,-p" help:"port"`
	Level   string `ask:"-l" help:"log level"`
	Name    string `ask:"--name" help:"name"`
	args    []string
	set     []string
}

func (c *shortCmd) Run(ctx context.Context) error {
	c.args = Args(ctx)
	for _, k := range []string{"verbose", "port", "l", "name"} {
		if IsSet(ctx, k) {
			c.set = append(c.set, k)
		}
	}
	return nil
}

func TestRun_shorthands(t *testing.T) {
	cmd := new(shortCmd)
	if err := Run(context.Background(), cmd, []string{"-vp", "80", "-l", "debug", "rest", "-v"}); err != nil {
		t.Fatal(err)
	}
	if !cmd.Verbose || cmd.Port != 80 || cmd.Level != "debug" {
		t.Fatalf("unexpected flags: %+v", cmd)
	}
	if got := strings.Join(cmd.args, " "); got != "rest -v" {
		t.Fatalf("unexpected remaining args: %q", got)
	}
	if got := strings.Join(cmd.set, " "); got != "verbose port l" {
		t.Fatalf("unexpected set flags: %q", got)
	}

	err := Run(context.Background(), new(shortCmd), []string{"-h"})
	if !errors.Is(err, HelpErr) {
		t.Fatalf("expected help error, got %v", err)
	}
	usage := UsageFromErr(err)
	for _, want := range []string{"-v --verbose ", "-p --port ", "-l ", "--name "} {
		if !strings.Contains(usage, want) {
			t.Errorf("expected usage to contain %q, got:\n%s", want, usage)
		}
	}
}

// Flag value errors are reported with the value, and retain the cause.
func TestRun_applyErrors(t *testing.T) {
	for _, args := range [][]string{{"--port=nope"}, {"--port", "nope"}, {"-p", "nope"}, {"-pnope"}, {"-p=nope"}} {
		err := Run(context.Background(), new(shortCmd), args)
		if err == nil {
			t.Fatalf("%v: expected error", args)
		}
		var numErr *strconv.NumError
		if !errors.As(err, &numErr) {
			t.Fatalf("%v: expected parse error cause, got: %v", args, err)
		}
		if !strings.Contains(err.Error(), `"nope"`) {
			t.Fatalf("%v: expected error to mention the value, got: %v", args, err)
		}
		if !strings.Contains(err.Error(), "port") {
			t.Fatalf("%v: expected error to mention the flag, got: %v", args, err)
		}
	}
}

type deprecatedCmd struct {
	Old string `ask:"--old -o" deprecated:"use --new"`
	New string `ask:"--new -n"`
}

func (c *deprecatedCmd) Run(ctx context.Context) error {
	return nil
}

// The error of the OnDeprecated callback is propagated, for long and short flags alike.
func TestRun_onDeprecatedError(t *testing.T) {
	errNoDeprecated := errors.New("deprecated flags are not allowed")
	var seen []string
	opt := OnDeprecated(func(ctx context.Context, fl PrefixedFlag) error {
		seen = append(seen, fl.Path)
		return errNoDeprecated
	})
	for _, args := range [][]string{{"--old=1"}, {"-o", "1"}, {"-o1"}} {
		seen = nil
		err := Run(context.Background(), new(deprecatedCmd), args, opt)
		if !errors.Is(err, errNoDeprecated) {
			t.Fatalf("%v: expected deprecation error, got: %v", args, err)
		}
		if strings.Join(seen, ",") != "old" {
			t.Fatalf("%v: expected callback for old flag, got %v", args, seen)
		}
	}
	seen = nil
	if err := Run(context.Background(), new(deprecatedCmd), []string{"-n", "1"}, opt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seen) != 0 {
		t.Fatalf("expected no callback, got %v", seen)
	}
}

type connOpts struct {
	Port uint16 `ask:"--port -p"`
}

type collidingShortCmd struct {
	A connOpts `ask:".a"`
	B connOpts `ask:".b"`
}

func (c *collidingShortCmd) Run(ctx context.Context) error {
	return nil
}

type collidingPathCmd struct {
	Port uint16   `ask:"--port"`
	Opts connOpts `ask:"."`
}

func (c *collidingPathCmd) Run(ctx context.Context) error {
	return nil
}

func TestRun_collisions(t *testing.T) {
	err := Run(context.Background(), new(collidingShortCmd), nil)
	if err == nil || !strings.Contains(err.Error(), `share shorthand -p`) {
		t.Fatalf("expected shorthand collision error, got: %v", err)
	}
	err = Run(context.Background(), new(collidingPathCmd), nil)
	if err == nil || !strings.Contains(err.Error(), `"port" is declared more than once`) {
		t.Fatalf("expected path collision error, got: %v", err)
	}
}

type valueCmd struct {
	X int `ask:"--x"`
}

func (c valueCmd) Run(ctx context.Context) error {
	return nil
}

type emptyValueCmd struct{}

func (c emptyValueCmd) Run(ctx context.Context) error {
	return nil
}

type unexportedFieldCmd struct {
	x int `ask:"--x"`
}

func (c *unexportedFieldCmd) Run(ctx context.Context) error {
	return nil
}

type unexportedOpts struct {
	Tag string `ask:"--tag"`
}

type unexportedGroupCmd struct {
	unexportedOpts `ask:".o"`
}

func (c *unexportedGroupCmd) Run(ctx context.Context) error {
	return nil
}

type unexportedDefaultOpts struct {
	Tag string `ask:"--tag"`
}

func (o *unexportedDefaultOpts) Default() {
	o.Tag = "default"
}

type unexportedDefaultGroupCmd struct {
	unexportedDefaultOpts `ask:".o"`
}

func (c *unexportedDefaultGroupCmd) Run(ctx context.Context) error {
	return nil
}

func TestRun_invalidCommands(t *testing.T) {
	ctx := context.Background()
	err := Run(ctx, (*shortCmd)(nil), nil)
	if err == nil || !strings.Contains(err.Error(), "nil command") {
		t.Fatalf("expected nil command error, got: %v", err)
	}
	err = Run(ctx, valueCmd{}, nil)
	if err == nil || !strings.Contains(err.Error(), "not addressable") {
		t.Fatalf("expected not addressable error, got: %v", err)
	}
	// a command without flags does not need to be a pointer
	if err := Run(ctx, emptyValueCmd{}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = Run(ctx, new(unexportedFieldCmd), nil)
	if err == nil || !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("expected not accessible error, got: %v", err)
	}
	// exported fields of an embedded unexported group are accessible
	cmd := new(unexportedGroupCmd)
	if err := Run(ctx, cmd, []string{"--o.tag=x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Tag != "x" {
		t.Fatalf("expected tag to be set, got %q", cmd.Tag)
	}
	// but the group itself is not, so its Default cannot be applied
	err = Run(ctx, new(unexportedDefaultGroupCmd), nil)
	if err == nil || !strings.Contains(err.Error(), "cannot apply Default") {
		t.Fatalf("expected default error, got: %v", err)
	}
}

type positionalCmd struct {
	Req  string `ask:"<req> -r"`
	Req2 uint8  `ask:"<req2>"`
	Opt  string `ask:"[opt]"`
	Grp  struct {
		Req3 string `ask:"<req3>"`
	} `ask:".grp"`
	args []string
	set  []string
}

func (c *positionalCmd) Run(ctx context.Context) error {
	c.args = Args(ctx)
	for _, k := range []string{"req", "req2", "opt", "grp.req3"} {
		if IsSet(ctx, k) {
			c.set = append(c.set, k)
		}
	}
	return nil
}

func TestRun_positionalArgs(t *testing.T) {
	env := map[string]string{}
	ctx := WithEnvFn(context.Background(), func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	})
	run := func(t *testing.T, args ...string) *positionalCmd {
		t.Helper()
		cmd := new(positionalCmd)
		if err := Run(ctx, cmd, args); err != nil {
			t.Fatal(err)
		}
		return cmd
	}
	check := func(t *testing.T, cmd *positionalCmd, req string, req2 uint8, opt, req3, rest, set string) {
		t.Helper()
		if cmd.Req != req || cmd.Req2 != req2 || cmd.Opt != opt || cmd.Grp.Req3 != req3 {
			t.Fatalf("unexpected positional values: %+v", cmd)
		}
		if got := strings.Join(cmd.args, " "); got != rest {
			t.Fatalf("unexpected remaining args: %q", got)
		}
		if got := strings.Join(cmd.set, " "); got != set {
			t.Fatalf("unexpected set flags: %q", got)
		}
	}

	t.Run("all positional", func(t *testing.T) {
		env = nil
		check(t, run(t, "a", "2", "c", "d", "e"), "a", 2, "d", "c", "e", "req req2 opt grp.req3")
		check(t, run(t, "a", "2", "c"), "a", 2, "", "c", "", "req req2 grp.req3")
	})
	t.Run("as named flags", func(t *testing.T) {
		env = nil
		// a positional arg set as named flag is skipped, the args fill the next positional args
		check(t, run(t, "--req2=2", "a", "c", "d"), "a", 2, "d", "c", "", "req req2 opt grp.req3")
		check(t, run(t, "-r", "a", "--grp.req3", "c", "2", "d", "e"), "a", 2, "d", "c", "e", "req req2 opt grp.req3")
		check(t, run(t, "--opt=d", "a", "2", "c", "e"), "a", 2, "d", "c", "e", "req req2 opt grp.req3")
		// all set as named flags: nothing is positional anymore
		check(t, run(t, "--req=a", "--req2=2", "--grp.req3=c", "--opt=d", "e"), "a", 2, "d", "c", "e", "req req2 opt grp.req3")
	})
	t.Run("from env", func(t *testing.T) {
		env = map[string]string{"REQ": "env-a", "GRP_REQ3": "env-c", "OPT": "env-d"}
		check(t, run(t, "2"), "env-a", 2, "env-d", "env-c", "", "req req2 opt grp.req3")
		check(t, run(t, "2", "e"), "env-a", 2, "env-d", "env-c", "e", "req req2 opt grp.req3")
		// named flags take precedence over env vars
		check(t, run(t, "--req=a", "2"), "a", 2, "env-d", "env-c", "", "req req2 opt grp.req3")
		env["REQ2"] = "x"
		if err := Run(ctx, new(positionalCmd), []string{"2"}); err == nil || !strings.Contains(err.Error(), `env var "REQ2"`) {
			t.Fatalf("expected env parse error, got: %v", err)
		}
	})
	t.Run("missing", func(t *testing.T) {
		env = nil
		err := Run(ctx, new(positionalCmd), []string{"a"})
		if err == nil || !strings.Contains(err.Error(), "missing required arguments: req2, grp.req3") {
			t.Fatalf("expected missing args error, got: %v", err)
		}
		err = Run(ctx, new(positionalCmd), []string{"--req2=2", "a"})
		if err == nil || !strings.Contains(err.Error(), "missing required arguments: grp.req3") {
			t.Fatalf("expected missing args error, got: %v", err)
		}
		env = map[string]string{"REQ": "a", "GRP_REQ3": "c"}
		err = Run(ctx, new(positionalCmd), nil)
		if err == nil || !strings.Contains(err.Error(), "missing required arguments: req2") {
			t.Fatalf("expected missing args error, got: %v", err)
		}
		if err := Run(ctx, new(positionalCmd), []string{"2"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("errors", func(t *testing.T) {
		env = nil
		err := Run(ctx, new(positionalCmd), []string{"a", "x", "c"})
		var numErr *strconv.NumError
		if !errors.As(err, &numErr) {
			t.Fatalf("expected parse error, got: %v", err)
		}
		// help takes precedence over missing args
		if err := Run(ctx, new(positionalCmd), []string{"--help"}); !errors.Is(err, HelpErr) {
			t.Fatalf("expected help error, got: %v", err)
		}
	})
}
