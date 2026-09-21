package ask

import (
	"errors"
	"flag"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestParseFlagDecl(t *testing.T) {
	cases := []struct {
		tag     string
		want    flagDecl
		wantErr string
	}{
		{tag: "--verbose", want: flagDecl{name: "verbose"}},
		{tag: "-v", want: flagDecl{name: "v", shorthand: 'v'}},
		{tag: "--verbose -v", want: flagDecl{name: "verbose", shorthand: 'v'}},
		{tag: "--verbose,-v", want: flagDecl{name: "verbose", shorthand: 'v'}},
		{tag: "--verbose, -v", want: flagDecl{name: "verbose", shorthand: 'v'}},
		{tag: "-v --verbose", want: flagDecl{name: "verbose", shorthand: 'v'}},
		{tag: "-v,--verbose", want: flagDecl{name: "verbose", shorthand: 'v'}},
		{tag: "  --verbose   -v  ", want: flagDecl{name: "verbose", shorthand: 'v'}},
		{tag: "--my-flag.x", want: flagDecl{name: "my-flag.x"}},
		{tag: "--a", want: flagDecl{name: "a"}},
		{tag: "-h", want: flagDecl{name: "h", shorthand: 'h'}},
		{tag: "<input>", want: flagDecl{name: "input", isArg: true, required: true}},
		{tag: "[output]", want: flagDecl{name: "output", isArg: true, required: false}},
		{tag: "<a>", want: flagDecl{name: "a", isArg: true, required: true}},
		{tag: "<a> -a", want: flagDecl{name: "a", shorthand: 'a', isArg: true, required: true}},
		{tag: "-b,[b]", want: flagDecl{name: "b", shorthand: 'b', isArg: true, required: false}},

		{tag: "", wantErr: "empty flag/arg declaration"},
		{tag: " , ", wantErr: "empty flag/arg declaration"},
		{tag: "-", wantErr: "must have a 1 char short name"},
		{tag: "-ab", wantErr: "must have a 1 char short name"},
		{tag: "-=", wantErr: "invalid short flag"},
		{tag: "--", wantErr: "at least 1 char name"},
		{tag: "<>", wantErr: "at least 1 char name"},
		{tag: "[]", wantErr: "at least 1 char name"},
		{tag: "-v -x", wantErr: "two different short-flag declarations"},
		{tag: "--a --b", wantErr: "different flag/arg declarations"},
		{tag: "--a <b>", wantErr: "different flag/arg declarations"},
		{tag: "<a> [b]", wantErr: "different flag/arg declarations"},
		{tag: "---a", wantErr: "invalid flag/arg name"},
		{tag: "--a=b", wantErr: "invalid flag/arg name"},
		{tag: "verbose", wantErr: "invalid flag/arg declaration"},
		{tag: "<a", wantErr: "invalid flag/arg declaration"},
		{tag: "a]", wantErr: "invalid flag/arg declaration"},
	}
	for _, tc := range cases {
		t.Run(tc.tag, func(t *testing.T) {
			got, err := parseFlagDecl(tc.tag)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got decl %+v", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// loadTestField loads the named field of the struct that v points to.
func loadTestField(t *testing.T, v any, fieldName string) (*Flag, error) {
	t.Helper()
	val := reflect.ValueOf(v).Elem()
	f, ok := val.Type().FieldByName(fieldName)
	if !ok {
		t.Fatalf("no field %q", fieldName)
	}
	return LoadField(f, val.FieldByIndex(f.Index))
}

func mustLoadTestField(t *testing.T, v any, fieldName string) *Flag {
	t.Helper()
	fl, err := loadTestField(t, v, fieldName)
	if err != nil {
		t.Fatalf("failed to load field %q: %v", fieldName, err)
	}
	if fl == nil {
		t.Fatalf("expected flag for field %q", fieldName)
	}
	return fl
}

type testPort uint16
type testID uint64
type testLabel string

type testCustomFlag struct {
	v string
}

func (c *testCustomFlag) Set(s string) error {
	c.v = "custom:" + s
	return nil
}

func (c *testCustomFlag) String() string {
	return c.v
}

var _ flag.Value = (*testCustomFlag)(nil)

func TestLoadField(t *testing.T) {
	type fields struct {
		NoTag       string
		Basic       uint64            `ask:"--basic" help:"basic help" deprecated:"use other" hidden:"" env:"BASIC_ENV"`
		Named       testPort          `ask:"--named"`
		NamedSlice  []testID          `ask:"--ids"`
		LabelSlice  []testLabel       `ask:"--labels"`
		Slice       []int8            `ask:"--slice"`
		Ptr         *uint64           `ask:"--ptr"`
		PtrCustom   *testCustomFlag   `ask:"--ptr-custom"`
		Custom      testCustomFlag    `ask:"--custom"`
		Positional  string            `ask:"<pos>"`
		PositionalX string            `ask:"<posx>" env:"-"`
		BadEnv      string            `ask:"--bad-env" env:""`
		ArgEnv      string            `ask:"<arg-env>" env:"ARG_ENV"`
		BadDecl     string            `ask:"nope"`
		BadType     map[string]string `ask:"--bad-type"`
		unexported  string            `ask:"--unexported"`
	}
	v := &fields{Basic: 42, Named: 7}

	t.Run("no tag", func(t *testing.T) {
		fl, err := loadTestField(t, v, "NoTag")
		if err != nil || fl != nil {
			t.Fatalf("expected no flag and no error, got %v, %v", fl, err)
		}
	})
	t.Run("basic", func(t *testing.T) {
		fl := mustLoadTestField(t, v, "Basic")
		if fl.Name != "basic" || fl.Help != "basic help" || fl.Deprecated != "use other" || !fl.Hidden || fl.Env != "BASIC_ENV" {
			t.Fatalf("unexpected flag: %+v", fl)
		}
		if fl.Default != "42" {
			t.Fatalf("expected default of current value, got %q", fl.Default)
		}
		if err := fl.Value.Set("0x10"); err != nil {
			t.Fatal(err)
		}
		if v.Basic != 16 {
			t.Fatalf("expected field to be set, got %d", v.Basic)
		}
		if _, ok := fl.Value.(*Uint64Value); !ok {
			t.Fatalf("expected built-in value type, got %T", fl.Value)
		}
	})
	t.Run("named basic type", func(t *testing.T) {
		fl := mustLoadTestField(t, v, "Named")
		if fl.Default != "7" {
			t.Fatalf("expected default 7, got %q", fl.Default)
		}
		if typ := fl.Value.(TypedValue).Type(); typ != "uint16" {
			t.Fatalf("expected uint16 type, got %q", typ)
		}
		if err := fl.Value.Set("80"); err != nil {
			t.Fatal(err)
		}
		if v.Named != 80 {
			t.Fatalf("expected field to be set, got %d", v.Named)
		}
		if err := fl.Value.Set("70000"); err == nil {
			t.Fatal("expected out of range error")
		}
	})
	t.Run("slice of named type", func(t *testing.T) {
		fl := mustLoadTestField(t, v, "NamedSlice")
		if typ := fl.Value.(TypedValue).Type(); typ != "uint64Slice" {
			t.Fatalf("expected uint64Slice type, got %q", typ)
		}
		if err := fl.Value.Set("1,0x2,3"); err != nil {
			t.Fatal(err)
		}
		if want := []testID{1, 2, 3}; !reflect.DeepEqual(v.NamedSlice, want) {
			t.Fatalf("expected %v, got %v", want, v.NamedSlice)
		}
		if s := fl.Value.String(); s != "1,2,3" {
			t.Fatalf("unexpected string: %q", s)
		}
		if err := fl.Value.Set("1,x"); err == nil {
			t.Fatal("expected parse error")
		}
		var numErr *strconv.NumError
		if err := fl.Value.Set("-1"); !errors.As(err, &numErr) {
			t.Fatalf("expected parse error, got %v", err)
		}
	})
	t.Run("slice of named string type", func(t *testing.T) {
		fl := mustLoadTestField(t, v, "LabelSlice")
		if typ := fl.Value.(TypedValue).Type(); typ != "stringSlice" {
			t.Fatalf("expected stringSlice type, got %q", typ)
		}
		if err := fl.Value.Set("a,b"); err != nil {
			t.Fatal(err)
		}
		if want := []testLabel{"a", "b"}; !reflect.DeepEqual(v.LabelSlice, want) {
			t.Fatalf("expected %v, got %v", want, v.LabelSlice)
		}
	})
	t.Run("slice of basic type", func(t *testing.T) {
		fl := mustLoadTestField(t, v, "Slice")
		if _, ok := fl.Value.(*Int8SliceValue); !ok {
			t.Fatalf("expected built-in slice value type, got %T", fl.Value)
		}
		if err := fl.Value.Set("-1,2"); err != nil {
			t.Fatal(err)
		}
		if want := []int8{-1, 2}; !reflect.DeepEqual(v.Slice, want) {
			t.Fatalf("expected %v, got %v", want, v.Slice)
		}
	})
	t.Run("nil pointer", func(t *testing.T) {
		fl := mustLoadTestField(t, v, "Ptr")
		if v.Ptr == nil {
			t.Fatal("expected pointer to be allocated")
		}
		if err := fl.Value.Set("5"); err != nil {
			t.Fatal(err)
		}
		if *v.Ptr != 5 {
			t.Fatalf("expected pointer target to be set, got %d", *v.Ptr)
		}
	})
	t.Run("nil pointer to custom flag value", func(t *testing.T) {
		fl := mustLoadTestField(t, v, "PtrCustom")
		if v.PtrCustom == nil {
			t.Fatal("expected pointer to be allocated")
		}
		if err := fl.Value.Set("x"); err != nil {
			t.Fatal(err)
		}
		if v.PtrCustom.v != "custom:x" {
			t.Fatalf("expected custom flag to be set, got %q", v.PtrCustom.v)
		}
	})
	t.Run("custom flag value", func(t *testing.T) {
		fl := mustLoadTestField(t, v, "Custom")
		if err := fl.Value.Set("y"); err != nil {
			t.Fatal(err)
		}
		if v.Custom.v != "custom:y" {
			t.Fatalf("expected custom flag to be set, got %q", v.Custom.v)
		}
	})
	t.Run("positional", func(t *testing.T) {
		fl := mustLoadTestField(t, v, "Positional")
		if !fl.IsArg || !fl.Required || fl.Name != "pos" {
			t.Fatalf("unexpected flag: %+v", fl)
		}
		fl = mustLoadTestField(t, v, "PositionalX")
		if fl.Env != "-" {
			t.Fatalf("expected env to be disabled, got %q", fl.Env)
		}
		fl = mustLoadTestField(t, v, "ArgEnv")
		if fl.Env != "ARG_ENV" {
			t.Fatalf("expected env to be set, got %q", fl.Env)
		}
	})
	for _, tc := range []struct{ field, wantErr string }{
		{"BadEnv", "cannot be empty"},
		{"BadDecl", "invalid ask declaration"},
		{"BadType", "unrecognized type"},
		{"unexported", "not accessible"},
	} {
		t.Run("error "+tc.field, func(t *testing.T) {
			_, err := loadTestField(t, v, tc.field)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
	t.Run("not addressable", func(t *testing.T) {
		val := reflect.ValueOf(*v)
		f, _ := val.Type().FieldByName("Basic")
		_, err := LoadField(f, val.FieldByIndex(f.Index))
		if err == nil || !strings.Contains(err.Error(), "not addressable") {
			t.Fatalf("expected not addressable error, got: %v", err)
		}
	})
}

func TestFlagValue_interfaceField(t *testing.T) {
	type fields struct {
		Iface flag.Value `ask:"--iface"`
	}
	v := new(fields)
	if _, err := loadTestField(t, v, "Iface"); err == nil || !strings.Contains(err.Error(), "nil interface") {
		t.Fatalf("expected nil interface error, got: %v", err)
	}
	v.Iface = new(testCustomFlag)
	fl := mustLoadTestField(t, v, "Iface")
	if err := fl.Value.Set("z"); err != nil {
		t.Fatal(err)
	}
	if got := v.Iface.String(); got != "custom:z" {
		t.Fatalf("expected pre-configured flag value to be set, got %q", got)
	}
}

func TestFlagValue_unsupportedTypes(t *testing.T) {
	type fields struct {
		Maps   []map[string]string `ask:"--maps"`
		Arrays [][2]string         `ask:"--arrays"`
		Array  [2]string           `ask:"--array"`
	}
	v := new(fields)
	for _, tc := range []struct{ field, wantErr string }{
		{"Maps", "unrecognized slice element type"},
		{"Arrays", "unrecognized element type of array-element slice"},
		{"Array", "unrecognized array element type"},
	} {
		_, err := loadTestField(t, v, tc.field)
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Fatalf("%s: expected error containing %q, got: %v", tc.field, tc.wantErr, err)
		}
	}
}
