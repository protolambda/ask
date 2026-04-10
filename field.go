package ask

import (
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

// LoadField loads a struct field as flag
func LoadField(f reflect.StructField, val reflect.Value) (fl *Flag, err error) {
	if !val.CanAddr() {
		return
	}
	v, ok := getAsk(&f)
	if !ok {
		return
	}
	name := ""
	shorthand := uint8(0)
	deprecated := ""
	help := ""
	hidden := false
	isArg := false
	required := false
	env := ""

	if h, ok := f.Tag.Lookup("help"); ok {
		help = h
	}

	// refers to the new value to use
	if d, ok := f.Tag.Lookup("deprecated"); ok {
		deprecated = d
	}
	if _, ok := f.Tag.Lookup("hidden"); ok {
		hidden = true
	}

	if v, ok := f.Tag.Lookup("env"); ok {
		if v == "" {
			return nil, fmt.Errorf("env key of field %s cannot be empty", f.Name)
		}
		env = v
	}

	value, err := FlagValue(f.Type, val)
	if err != nil {
		return nil, fmt.Errorf("failed to handle value type of field %s as flag/arg: %v", f.Name, err)
	}

	for _, k := range strings.Split(v, " ") {
		if k == "" {
			continue
		}
		if name != "" {
			return nil, fmt.Errorf("field %q cannot have different flag/arg declarations", f.Name)
		}
		if strings.HasPrefix(k, "--") {
			if len(k) < 3 {
				return nil, fmt.Errorf("field %q long flag must have at least 1 char name", f.Name)
			}
			name = k[2:]
			continue
		}
		if strings.HasPrefix(k, "-") {
			if shorthand != 0 {
				return nil, fmt.Errorf("field %q cannot have two different short-flag style declarations", f.Name)
			}
			if len(k) == 2 {
				return nil, fmt.Errorf("field %q short flag must have a 1 char short name", f.Name)
			}
			shorthand = k[1]
			continue
		}
		if len(v) < 3 {
			return nil, fmt.Errorf("field %q positional arg must have at least 1 char name", f.Name)
		}
		if strings.HasPrefix(v, "<") && strings.HasSuffix(v, ">") {
			name = v[1 : len(v)-1]
			isArg = true
			required = true
			continue
		}
		if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
			name = v[1 : len(v)-1]
			isArg = true
			continue
		}
		return nil, fmt.Errorf("struct field %q has invalid Ask arg/flag declaration", f.Name)
	}

	// use shorthand as name if name is missing
	if shorthand != 0 && name == "" {
		name = string(shorthand)
	}

	return &Flag{
		Value:      value,
		Name:       name,
		Shorthand:  shorthand,
		Env:        env,
		IsArg:      isArg,
		Help:       help,
		Default:    value.String(),
		Required:   required,
		Deprecated: deprecated,
		Hidden:     hidden,
	}, nil
}
