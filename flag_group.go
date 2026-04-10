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

// Load adds all flags inferred from the given value to the group.
func (grp *FlagGroup) Load(val reflect.Value) error {
	return fillGroup(grp, val)
}

func fillGroup(grp *FlagGroup, val reflect.Value) error {
	typ := val.Type()
	if grp.Help == nil && typ.Implements(helpType) {
		grp.Help = val.Interface().(Help)
	}
	if typ.Implements(initDefaultType) {
		val.Interface().(InitDefault).Default()
	}
	switch val.Kind() {
	case reflect.Struct:
		fieldCount := val.NumField()
		for i := 0; i < fieldCount; i++ {
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

			// recurse into explicitly inline-squashed fields
			if tag == "." {
				if err := fillGroup(grp, v.Addr()); err != nil {
					return fmt.Errorf("failed to load squashed flag group into group %q: %v", grp.GroupName, err)
				}
				continue
			}

			// recurse into sub-groups
			if strings.HasPrefix(tag, ".") {
				subGrp := &FlagGroup{GroupName: tag[1:]}
				err := subGrp.Load(v.Addr())
				if err != nil {
					return err
				}
				if h, ok := f.Tag.Lookup("help"); ok {
					subGrp.Help = inlineHelp(h)
				}
				grp.Entries = append(grp.Entries, subGrp)
				continue
			}

			// handle individual fields
			fl, err := LoadField(typ.Field(i), v)
			if err != nil {
				return err
			}
			grp.Flags = append(grp.Flags, fl)
			continue
		}
		return nil
	case reflect.Ptr:
		if val.IsNil() {
			val.Set(reflect.New(val.Type().Elem()))
		}
		return fillGroup(grp, val.Elem())
	default:
		return fmt.Errorf("type %T, is not a valid group of flags", typ)
	}
}
