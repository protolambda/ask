package ask

import (
	"fmt"
	"reflect"
	"strings"
)

// inlineHelp is a util to turn a string into an object that implements Help,
// used to e.g. turn struct-tag values on field groups into deferred Help info.
type inlineHelp string

func (v inlineHelp) Help() string {
	return string(v)
}

type FlagGroup struct {
	GroupName string
	// Optional help info, provided by the struct that covers this group of flags
	Help
	// sub-groups
	Entries []*FlagGroup
	// flags in this group (does not include sub-groups)
	Flags []*Flag
}

func (g *FlagGroup) Usage(prefix string, showHidden bool, out *strings.Builder) {
	path := g.path(prefix)
	if g.GroupName != "" {
		out.WriteString("# ")
		out.WriteString(path)
		out.WriteString("\n")
	}
	if g.Help != nil {
		out.WriteString(g.Help.Help())
		out.WriteString("\n\n")
	}
	for _, f := range g.Flags {
		if f.Hidden && !showHidden {
			continue
		}
		out.WriteString("  ")
		indent := 2
		if f.Shorthand != 0 {
			out.WriteString("-")
			out.WriteByte(f.Shorthand)
			out.WriteString(" ")
			// e.g. "-c "
			indent += 1 + 1 + 1
		}
		if f.Name != string(f.Shorthand) {
			var prefix, suffix string
			if f.IsArg {
				if f.Required {
					prefix = "<"
					suffix = ">"
				} else {
					prefix = "["
					suffix = "]"
				}
			} else {
				prefix = "--"
			}
			out.WriteString(prefix)
			if path != "" {
				out.WriteString(path)
				out.WriteString(".")
				indent += len(path) + 1
			}
			out.WriteString(f.Name)
			out.WriteString(suffix)
			out.WriteString(" ")
			indent += len(prefix) + len(f.Name) + len(suffix) + 1
		}
		if indent < 30 {
			out.WriteString(strings.Repeat(" ", 30-indent))
		}
		out.WriteString(f.Help)
		if f.Default != "" {
			out.WriteString(" (default: ")
			out.WriteString(f.Default)
			out.WriteString(")")
		}
		if tv, ok := f.Value.(TypedValue); ok {
			typ := tv.Type()
			if typ != "" {
				out.WriteString(" (type: ")
				out.WriteString(typ)
				out.WriteString(")")
			}
		}
		if f.Env != "-" {
			envKey := f.Env
			if envKey == "" {
				fPath := f.Name
				if path != "" {
					fPath = path + "." + f.Name
				}
				envKey = FlagPathToEnvKey(fPath)
			}
			out.WriteString(" (env: ")
			out.WriteString(envKey)
			out.WriteString(")")
		}
		if f.Deprecated != "" {
			out.WriteString(" DEPRECATED: ")
			out.WriteString(f.Deprecated)
		}
		out.WriteString("\n")
	}
	out.WriteString("\n")
	for _, e := range g.Entries {
		e.Usage(path, showHidden, out)
	}
}

func (g *FlagGroup) path(prefix string) string {
	path := prefix
	if g.GroupName != "" {
		if prefix == "" {
			path = g.GroupName
		} else {
			path = prefix + "." + g.GroupName
		}
	}
	return path
}

func (g *FlagGroup) All(prefix string) []PrefixedFlag {
	out := make([]PrefixedFlag, 0, len(g.Flags))
	g.all(&out, prefix)
	return out
}

func (g *FlagGroup) all(out *[]PrefixedFlag, prefix string) {
	path := g.path(prefix)
	for _, f := range g.Flags {
		k := f.Name
		if path != "" {
			k = path + "." + f.Name
		}
		*out = append(*out, PrefixedFlag{Path: k, Flag: f})
	}
	for _, g := range g.Entries {
		g.all(out, path)
	}
}

// Load applies the defaults of the given value (see InitDefault),
// and then adds all flags inferred from the value to the group.
// Nil pointers to flag groups and flag values are allocated.
func (grp *FlagGroup) Load(val reflect.Value) error {
	if err := applyDefaults(val); err != nil {
		return err
	}
	return fillGroup(grp, val)
}

// derefAlloc follows pointers, allocating nil ones, to get to the value they point to.
func derefAlloc(val reflect.Value) (reflect.Value, error) {
	for val.Kind() == reflect.Ptr {
		if val.IsNil() {
			if !val.CanSet() {
				return val, fmt.Errorf("cannot allocate nil %s: not settable", val.Type())
			}
			val.Set(reflect.New(val.Type().Elem()))
		}
		val = val.Elem()
	}
	return val, nil
}

// accessible returns the value as interface, through a pointer if addressable,
// so that methods with pointer receivers can be found. If the value cannot be accessed
// through reflection (i.e. it comes from an unexported field), ok is false.
func accessible(val reflect.Value) (v any, ok bool) {
	if val.CanAddr() {
		val = val.Addr()
	}
	if !val.CanInterface() {
		return nil, false
	}
	return val.Interface(), true
}

// applyDefaults applies the Default of every InitDefault in the tree of the given value, bottom-up:
// first the flag values of a struct, then the struct itself, and then the struct that embeds it, etc.
// Thus a struct has the final say over the defaults of its embedded groups and flag values.
// This runs before any flag is bound to a value, so a Default may replace pointer fields freely.
func applyDefaults(val reflect.Value) error {
	val, err := derefAlloc(val)
	if err != nil {
		return err
	}
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("type %s, is not a valid group of flags", val.Type())
	}
	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		tag, ok := getAsk(&f)
		// skip ignored fields
		if !ok || tag == "-" {
			continue
		}
		v := val.Field(i)
		if !v.CanAddr() {
			return fmt.Errorf("field %q is not addressable, the command must be passed as pointer", f.Name)
		}
		// recurse into inline-squashed fields and sub-groups
		if strings.HasPrefix(tag, ".") {
			if err := applyDefaults(v); err != nil {
				return err
			}
			continue
		}
		// individual flag/arg fields
		fv, err := derefAlloc(v)
		if err != nil {
			return fmt.Errorf("field %q: %w", f.Name, err)
		}
		if err := callDefault(fv); err != nil {
			return fmt.Errorf("field %q: %w", f.Name, err)
		}
	}
	return callDefault(val)
}

// callDefault calls Default on the value if it implements InitDefault.
func callDefault(val reflect.Value) error {
	v, ok := accessible(val)
	if !ok {
		if reflect.PtrTo(val.Type()).Implements(initDefaultType) {
			return fmt.Errorf("cannot apply Default of %s: value is not accessible (unexported field)", val.Type())
		}
		return nil
	}
	if d, ok := v.(InitDefault); ok {
		d.Default()
	}
	return nil
}

func fillGroup(grp *FlagGroup, val reflect.Value) error {
	val, err := derefAlloc(val)
	if err != nil {
		return err
	}
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("type %s, is not a valid group of flags", val.Type())
	}
	typ := val.Type()
	if v, ok := accessible(val); ok {
		if h, ok := v.(Help); ok && grp.Help == nil {
			grp.Help = h
		}
	} else if reflect.PtrTo(typ).Implements(helpType) {
		return fmt.Errorf("cannot use Help of %s: value is not accessible (unexported field)", typ)
	}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if _, ok := f.Tag.Lookup("changed"); ok {
			return fmt.Errorf("struct-tag 'changed' is not supported anymore")
		}

		tag, ok := getAsk(&f)
		// skip ignored fields
		if !ok || tag == "-" {
			continue
		}
		v := val.Field(i)
		if !v.CanAddr() {
			return fmt.Errorf("field %q is not addressable, the command must be passed as pointer", f.Name)
		}

		// recurse into explicitly inline-squashed fields
		if tag == "." {
			if err := fillGroup(grp, v); err != nil {
				return fmt.Errorf("failed to load squashed flag group into group %q: %w", grp.GroupName, err)
			}
			continue
		}

		// recurse into sub-groups
		if strings.HasPrefix(tag, ".") {
			subGrp := &FlagGroup{GroupName: tag[1:]}
			if err := fillGroup(subGrp, v); err != nil {
				return err
			}
			if h, ok := f.Tag.Lookup("help"); ok {
				subGrp.Help = inlineHelp(h)
			}
			grp.Entries = append(grp.Entries, subGrp)
			continue
		}

		// handle individual fields
		fl, err := LoadField(f, v)
		if err != nil {
			return err
		}
		grp.Flags = append(grp.Flags, fl)
	}
	return nil
}
