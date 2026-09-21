package ask

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"reflect"
	"strings"
	"time"
)

func getAsk(f *reflect.StructField) (v string, ok bool) {
	return f.Tag.Lookup("ask")
}

var typedFlagValueType = reflect.TypeOf((*TypedValue)(nil)).Elem()
var flagValueType = reflect.TypeOf((*flag.Value)(nil)).Elem()

var durationType = reflect.TypeOf(time.Second)
var ipType = reflect.TypeOf(net.IP{})
var ipmaskType = reflect.TypeOf(net.IPMask{})
var ipNetType = reflect.TypeOf(net.IPNet{})

// flagDecl is the parsed content of an "ask" struct tag of a flag or positional arg.
type flagDecl struct {
	// name is the long flag name (without "--"), or the arg name (without "<>" or "[]").
	// If only a shorthand is declared, the name is the shorthand.
	name      string
	shorthand uint8
	isArg     bool
	required  bool
}

func isDeclSeparator(r rune) bool {
	return r == ' ' || r == ','
}

// parseFlagDecl parses the flag/arg declarations of an "ask" struct tag.
// Declarations are separated by spaces and/or commas, e.g. "--verbose -v" or "--verbose,-v".
// Supported declarations:
//   - "--name": long flag
//   - "-c": shorthand flag (single character)
//   - "<name>": required positional arg
//   - "[name]": optional positional arg
//
// A name (long flag or positional arg) may be combined with one shorthand.
func parseFlagDecl(tag string) (flagDecl, error) {
	var d flagDecl
	for _, k := range strings.FieldsFunc(tag, isDeclSeparator) {
		switch {
		case strings.HasPrefix(k, "--"):
			if err := d.setName(k[2:], false, false); err != nil {
				return d, err
			}
		case strings.HasPrefix(k, "-"):
			if len(k) != 2 {
				return d, fmt.Errorf("short flag %q must have a 1 char short name", k)
			}
			if d.shorthand != 0 {
				return d, fmt.Errorf("cannot have two different short-flag declarations: %q and %q", string(d.shorthand), k)
			}
			if k[1] == '=' {
				return d, fmt.Errorf("invalid short flag %q", k)
			}
			d.shorthand = k[1]
		case strings.HasPrefix(k, "<") && strings.HasSuffix(k, ">"):
			if err := d.setName(k[1:len(k)-1], true, true); err != nil {
				return d, err
			}
		case strings.HasPrefix(k, "[") && strings.HasSuffix(k, "]"):
			if err := d.setName(k[1:len(k)-1], true, false); err != nil {
				return d, err
			}
		default:
			return d, fmt.Errorf("invalid flag/arg declaration %q", k)
		}
	}
	if d.name == "" && d.shorthand == 0 {
		return d, errors.New("empty flag/arg declaration")
	}
	// use shorthand as name if name is missing
	if d.name == "" {
		d.name = string(d.shorthand)
	}
	return d, nil
}

func (d *flagDecl) setName(name string, isArg, required bool) error {
	if d.name != "" {
		return fmt.Errorf("cannot have different flag/arg declarations: %q and %q", d.name, name)
	}
	if name == "" {
		return errors.New("flag/arg must have at least 1 char name")
	}
	if name[0] == '-' || strings.ContainsRune(name, '=') {
		return fmt.Errorf("invalid flag/arg name %q", name)
	}
	d.name = name
	d.isArg = isArg
	d.required = required
	return nil
}

// LoadField loads a struct field as flag or positional arg.
// A nil Flag without error is returned if the field has no "ask" struct tag.
// The field value must be addressable, and accessible through reflection (i.e. exported).
// The current value of the field is captured as default:
// defaults (see InitDefault) must be applied before loading, see FlagGroup.Load.
func LoadField(f reflect.StructField, val reflect.Value) (*Flag, error) {
	tag, ok := getAsk(&f)
	if !ok {
		return nil, nil
	}
	if !val.CanAddr() {
		return nil, fmt.Errorf("field %q is not addressable, the command must be passed as pointer", f.Name)
	}
	if !val.CanInterface() {
		return nil, fmt.Errorf("field %q is not accessible, flag fields must be exported", f.Name)
	}

	decl, err := parseFlagDecl(tag)
	if err != nil {
		return nil, fmt.Errorf("field %q has invalid ask declaration %q: %w", f.Name, tag, err)
	}

	env := ""
	if v, ok := f.Tag.Lookup("env"); ok {
		if v == "" {
			return nil, fmt.Errorf("env key of field %s cannot be empty", f.Name)
		}
		env = v
	}

	value, err := FlagValue(f.Type, val)
	if err != nil {
		return nil, fmt.Errorf("failed to handle value type of field %s as flag/arg: %w", f.Name, err)
	}

	_, hidden := f.Tag.Lookup("hidden")

	return &Flag{
		Value:      value,
		Name:       decl.name,
		Shorthand:  decl.shorthand,
		Env:        env,
		IsArg:      decl.isArg,
		Help:       f.Tag.Get("help"),
		Default:    value.String(),
		Required:   decl.required,
		Deprecated: f.Tag.Get("deprecated"), // refers to the new value to use
		Hidden:     hidden,
	}, nil
}
