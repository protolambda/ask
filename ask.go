package ask

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"reflect"
	"sort"
	"strings"
	"time"
	"unsafe"
)

var HelpErr = errors.New("ask: help asked with flag")

var UnrecognizedErr = errors.New("command was not recognized")

// TypedValue is the interface to the dynamic value stored in a flag.
// (The default value is represented as a string.)
// Extension of flag.Value with Type information.
type TypedValue interface {
	flag.Value
	Type() string
}

type ImplicitValue interface {
	flag.Value
	// Implicit returns the omitted value of the flag if the flag is used without explicit value
	Implicit() string
}

type Command interface {
	// Run the command, with context and remaining unrecognized args
	Run(ctx context.Context, args ...string) error
}

var commandType = reflect.TypeOf((*Command)(nil)).Elem()

type CommandRoute interface {
	// Cmd gets a sub-command, which can be a Command or CommandRoute
	// The command that is returned will be loaded with `Load` before it runs or its subcommand is retrieved.
	// Return nil if the command route should be ignored, e.g. if this route is also a regular command with arguments.
	Cmd(route string) (cmd interface{}, err error)
}

// CommandKnownRoutes may be implemented by a CommandRoute to declare which routes are accessible,
// useful for e.g. help messages to give more information for each of the subcommands.
type CommandKnownRoutes interface {
	// Routes lists the sub-commands that can be asked from Get.
	Routes() []string
}

var commandRouteType = reflect.TypeOf((*CommandRoute)(nil)).Elem()

type Help interface {
	// Help explains how a command or group of flags is used.
	Help() string
}

var helpType = reflect.TypeOf((*Help)(nil)).Elem()

// InitDefault can be implemented by a command to not rely on the parent command initializing the command correctly,
// and instead move the responsibility to the command itself.
// The default is initialized during Load, and a command may embed multiple sub-structures that implement Default.
type InitDefault interface {
	// Default the flags of a command.
	Default()
}

var initDefaultType = reflect.TypeOf((*InitDefault)(nil)).Elem()

type Flag struct {
	Value flag.Value
	Name  string
	// 0 if no shorthand
	Shorthand uint8
	IsArg     bool
	Help      string
	Default   string
	Required  bool
	// Reason for deprecation. Empty if not deprecated.
	Deprecated string
	Hidden     bool
}

type PrefixedFlag struct {
	// Prefix and flag name, segments separated by dot
	Path string
	*Flag
}

type InlineHelp string

func (v InlineHelp) Help() string {
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

// ChangedMarkers tracks which flags are changed.
type ChangedMarkers map[string][]*bool

// An interface{} can be loaded as a command-description to execute it. See Load()
type CommandDescription struct {
	FlagGroup
	// Define a field as 'MySettingChanged bool `changed:"my-setting"`' to e.g. track '--my-setting' being changed.
	// The same flag may be tracked with multiple fields
	ChangedMarkers ChangedMarkers
	// Command to run, may be nil if nothing has to run
	Command
	// Sub-command routing, can create commands (or other sub-commands) to access, may be nil if no sub-commands
	CommandRoute
}

// Load takes a structure instance that defines a command through its type,
// and the default values by determining them from the actual type.
func Load(val interface{}) (*CommandDescription, error) {
	return LoadReflect(reflect.ValueOf(val))
}

// LoadReflect is the same as Load, but directly using reflection to handle the value.
func LoadReflect(val reflect.Value) (*CommandDescription, error) {
	descr := &CommandDescription{
		ChangedMarkers: make(map[string][]*bool),
	}
	return descr, descr.LoadReflect(val)
}

// Load adds more flags/args/meta to the command description.
// It recursively goes into the field if it's tagged with `ask:"."` (recurse depth-first).
// Embedded fields are handled as regular fields unless explicitly squashed.
// It skips the field explicitly if it's tagged with `ask:"-"`
// Multiple target values can be loaded if they do not conflict, the first Command and CommandRoute found will be used.
// The flags will be set over all loaded values.
func (descr *CommandDescription) Load(val interface{}) error {
	return descr.LoadReflect(reflect.ValueOf(val))
}

// LoadReflect is the same as Load, but directly using reflection to handle the value.
func (descr *CommandDescription) LoadReflect(val reflect.Value) error {
	typ := val.Type()
	if descr.Command == nil && typ.Implements(commandType) {
		descr.Command = val.Interface().(Command)
	}
	if descr.CommandRoute == nil && typ.Implements(commandRouteType) {
		descr.CommandRoute = val.Interface().(CommandRoute)
	}
	grp, err := LoadGroup("", val, descr.ChangedMarkers)
	if err != nil {
		return err
	}
	descr.FlagGroup = *grp
	return nil
}

func LoadGroup(name string, val reflect.Value, changes ChangedMarkers) (*FlagGroup, error) {
	typ := val.Type()
	var grp FlagGroup
	grp.GroupName = name
	if typ.Implements(helpType) {
		grp.Help = val.Interface().(Help)
	}
	if err := fillGroup(&grp, val, changes); err != nil {
		return nil, err
	}
	return &grp, nil
}

func fillGroup(grp *FlagGroup, val reflect.Value, changes ChangedMarkers) error {
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
			if changed, ok := getChanged(&f); ok {
				v := val.Field(i)
				if !v.CanAddr() {
					return fmt.Errorf("cannot get address of changed flag boolean field '%s'", f.Name)
				}
				if ptr, ok := v.Addr().Interface().(*bool); ok {
					changes[changed] = append(changes[changed], ptr)
				} else {
					return fmt.Errorf("changed flag field '%s' is not a bool", f.Name)
				}
				continue
			}

			tag, ok := getAsk(&f)
			// skip ignored fields
			if !ok || tag == "-" {
				continue
			}
			v := val.Field(i)

			// recurse into explicitly inline-squashed fields
			if tag == "." {
				if err := fillGroup(grp, v.Addr(), changes); err != nil {
					return fmt.Errorf("failed to load squashed flag group into group %q: %v", grp.GroupName, err)
				}
				continue
			}

			// recurse into sub-groups
			if strings.HasPrefix(tag, ".") {
				subGrp, err := LoadGroup(tag[1:], v.Addr(), changes)
				if err != nil {
					return err
				}
				if h, ok := f.Tag.Lookup("help"); ok {
					subGrp.Help = InlineHelp(h)
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
		return fillGroup(grp, val.Elem(), changes)
	default:
		return fmt.Errorf("type %T, is not a valid group of flags", typ)
	}
}

// Usage prints the help information and the usage of all flags.
func (descr *CommandDescription) Usage(showHidden bool) string {
	var out strings.Builder
	out.WriteString("(command)")
	all := descr.All("")

	for _, a := range all {
		if a.IsArg && a.Required {
			out.WriteString(" <")
			out.WriteString(a.Path)
			out.WriteString(">")
		}
	}
	for _, a := range all {
		if a.IsArg && !a.Required {
			out.WriteString(" [")
			out.WriteString(a.Path)
			out.WriteString("]")
		}
	}
	flagCount := 0
	for _, a := range all {
		if !a.IsArg && (!a.Hidden || showHidden) {
			flagCount += 1
		}
	}
	if flagCount > 0 {
		out.WriteString(fmt.Sprintf(" # %d flags (see below)", flagCount))
	}

	out.WriteString("\n\n")

	if len(all) > 0 {
		descr.FlagGroup.Usage("", showHidden, &out)
		out.WriteString("\n")
	}

	if descr.CommandRoute != nil {
		knownRoutes, ok := descr.CommandRoute.(CommandKnownRoutes)
		if ok {
			out.WriteString("Sub commands:\n")
			routes := knownRoutes.Routes()
			maxRouteLen := 0
			for _, r := range routes {
				if len(r) > maxRouteLen {
					maxRouteLen = len(r)
				}
			}
			for _, k := range routes {
				out.WriteString("  ")
				out.WriteString(k)
				if len(k) < maxRouteLen {
					out.WriteString(strings.Repeat(" ", maxRouteLen-len(k)))
				}
				out.WriteString("  ")
				subCmd, err := descr.CommandRoute.Cmd(k)
				if err != nil {
					out.WriteString(err.Error())
				} else if subCmd == nil {
					out.WriteString("Command route not available")
				} else {
					subDescr, err := Load(subCmd)
					if err != nil {
						out.WriteString("[error] command is invalid\n")
						out.WriteString(err.Error())
					} else {
						if subDescr.Help != nil {
							out.WriteString(subDescr.Help.Help())
						}
						// no info in no help available but valid otherwise
					}
				}
				out.WriteString("\n")
			}
		}
	}

	return out.String()
}

type ExecutionOptions struct {
	OnDeprecated func(fl PrefixedFlag) error
}

// Execute runs the command, with given context and arguments.
// Commands may have routes to sub-commands, the final sub-command that actually runs is returned,
// and may be nil in case of an error.
//
// A HelpErr is returned when help information was requested for the command (through `help`, `--help` or `-h`)
// A UnrecognizedErr is returned when a sub-command was expected but not found.
//
// To add inputs/outputs such as STDOUT to a command, add the readers/writers as field in the command struct definition,
// and the command can pass them on to sub-commands. Similarly logging and other misc. data can be passed around.
// The execute parameters are kept minimal.
//
// opts.OnDeprecated is called for each deprecated flag,
// and command execution exits immediately if this callback returns an error.
func (descr *CommandDescription) Execute(ctx context.Context, opts *ExecutionOptions, args ...string) (final *CommandDescription, err error) {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		return descr, HelpErr
	}
	if opts == nil {
		opts = &ExecutionOptions{}
	}

	if descr.CommandRoute != nil && len(args) > 0 {
		sub, err := descr.CommandRoute.Cmd(args[0])
		if err != nil {
			return nil, err
		}
		if sub != nil {
			subCmd, err := Load(sub)
			if err != nil {
				return nil, err
			}
			return subCmd.Execute(ctx, opts, args[1:]...)
		}
		// deal with it as regular command if it is not recognized as sub-command
	}

	var long []PrefixedFlag
	var short []PrefixedFlag
	var positionalRequired []PrefixedFlag
	var positionalOptional []PrefixedFlag
	for _, pf := range descr.FlagGroup.All("") {
		if pf.IsArg {
			if pf.Required {
				positionalRequired = append(positionalRequired, pf)
			} else {
				positionalOptional = append(positionalOptional, pf)
			}
		} else {
			if pf.Shorthand != 0 {
				short = append(short, pf)
			}
			if string(pf.Shorthand) != pf.Name {
				long = append(long, pf)
			}
		}
	}
	sort.SliceStable(long, func(i, j int) bool {
		return long[i].Path < long[j].Path
	})
	sort.SliceStable(short, func(i, j int) bool {
		return short[i].Path < short[j].Path
	})

	seen := make(map[string]struct{})
	set := func(fl PrefixedFlag, value string) error {
		seen[fl.Path] = struct{}{}
		for _, ptr := range descr.ChangedMarkers[fl.Path] {
			*ptr = true
		}

		if fl.Deprecated != "" && opts.OnDeprecated != nil {
			if err := opts.OnDeprecated(fl); err != nil {
				return err
			}
		}

		return fl.Flag.Value.Set(value)
	}
	remaining, err := ParseArgs(short, long, args, set)
	if err != nil {
		// can be a HelpErr to indicate a help-flag was detected
		return descr, err
	}

	var remainingPositionalRequiredFlags []PrefixedFlag
	for _, v := range positionalRequired {
		if _, ok := seen[v.Path]; !ok {
			remainingPositionalRequiredFlags = append(remainingPositionalRequiredFlags, v)
		}
	}
	var remainingPositionalOptionalFlags []PrefixedFlag
	for _, v := range positionalOptional {
		if _, ok := seen[v.Path]; !ok {
			remainingPositionalOptionalFlags = append(remainingPositionalOptionalFlags, v)
		}
	}

	// process required args
	if len(remaining) < len(remainingPositionalRequiredFlags) {
		remainingPaths := make([]string, 0, len(remainingPositionalRequiredFlags))
		for _, pf := range remainingPositionalRequiredFlags {
			remainingPaths = append(remainingPaths, pf.Path)
		}
		return descr, fmt.Errorf("got %d arguments, but expected %d, missing required arguments: %s",
			len(remaining), len(remainingPositionalRequiredFlags), strings.Join(remainingPaths, ", "))
	}
	for i := range remainingPositionalRequiredFlags {
		if err := set(remainingPositionalRequiredFlags[i], remaining[i]); err != nil {
			return descr, err
		}
	}
	remaining = remaining[len(remainingPositionalRequiredFlags):]

	// process optional args
	if len(remainingPositionalOptionalFlags) > 0 {
		count := 0
		for i := range remaining {
			if i >= len(remainingPositionalOptionalFlags) {
				break
			}
			if err := set(remainingPositionalOptionalFlags[i], remaining[i]); err != nil {
				return descr, err
			}
			count += 1
		}
		remaining = remaining[count:]
	}

	if descr.Command != nil {
		err := descr.Command.Run(ctx, remaining...)
		return descr, err
	}

	return descr, UnrecognizedErr
}

func getAsk(f *reflect.StructField) (v string, ok bool) {
	return f.Tag.Lookup("ask")
}

func getChanged(f *reflect.StructField) (v string, ok bool) {
	return f.Tag.Lookup("changed")
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
		IsArg:      isArg,
		Help:       help,
		Default:    value.String(),
		Required:   required,
		Deprecated: deprecated,
		Hidden:     hidden,
	}, nil
}

func FlagValue(typ reflect.Type, val reflect.Value) (flag.Value, error) {
	// Get the pointer to the destination struct, to route pflags to
	ptr := unsafe.Pointer(val.Addr().Pointer())

	var fl flag.Value

	if typ.Implements(typedFlagValueType) {
		fl = val.Interface().(TypedValue)
	} else if reflect.PtrTo(typ).Implements(typedFlagValueType) {
		fl = val.Addr().Interface().(TypedValue)
	} else if typ.Implements(flagValueType) {
		fl = val.Interface().(flag.Value)
	} else if reflect.PtrTo(typ).Implements(flagValueType) {
		fl = val.Addr().Interface().(flag.Value)
	} else if typ == durationType {
		fl = (*DurationValue)(ptr)
	} else if typ == ipType {
		fl = (*IPValue)(ptr)
	} else if typ == ipNetType {
		fl = (*IPNetValue)(ptr)
	} else if typ == ipmaskType {
		fl = (*IPMaskValue)(ptr)
	} else {
		switch typ.Kind() {
		// unsigned integers
		case reflect.Uint:
			fl = (*UintValue)(ptr)
		case reflect.Uint8:
			fl = (*Uint8Value)(ptr)
		case reflect.Uint16:
			fl = (*Uint16Value)(ptr)
		case reflect.Uint32:
			fl = (*Uint32Value)(ptr)
		case reflect.Uint64:
			fl = (*Uint64Value)(ptr)
		// signed integers
		case reflect.Int:
			fl = (*IntValue)(ptr)
		case reflect.Int8:
			fl = (*Int8Value)(ptr)
		case reflect.Int16:
			fl = (*Int16Value)(ptr)
		case reflect.Int32:
			fl = (*Int32Value)(ptr)
		case reflect.Int64:
			fl = (*Int64Value)(ptr)
		// Misc
		case reflect.String:
			fl = (*StringValue)(ptr)
		case reflect.Bool:
			fl = (*BoolValue)(ptr)
		case reflect.Float32:
			fl = (*Float32Value)(ptr)
		case reflect.Float64:
			fl = (*Float64Value)(ptr)
		// Cobra commons
		case reflect.Slice:
			elemTyp := typ.Elem()
			if elemTyp == durationType {
				fl = (*DurationSliceValue)(ptr)
			} else if elemTyp == ipType {
				fl = (*IPSliceValue)(ptr)
			} else {
				switch elemTyp.Kind() {
				case reflect.Array:
					switch elemTyp.Elem().Kind() {
					case reflect.Uint8:
						fl = &fixedLenBytesSlice{Dest: val}
					default:
						return nil, fmt.Errorf("unrecognized element type of array-element slice: %v", elemTyp.Elem().String())
					}
				case reflect.Uint8:
					b := (*[]byte)(ptr)
					fl = (*BytesHexFlag)(b)
				case reflect.Uint16:
					fl = (*Uint16SliceValue)(ptr)
				case reflect.Uint32:
					fl = (*Uint32SliceValue)(ptr)
				case reflect.Uint64:
					fl = (*Uint64SliceValue)(ptr)
				case reflect.Uint:
					fl = (*UintSliceValue)(ptr)
				case reflect.Int8:
					fl = (*Int8SliceValue)(ptr)
				case reflect.Int16:
					fl = (*Int16SliceValue)(ptr)
				case reflect.Int32:
					fl = (*Int32SliceValue)(ptr)
				case reflect.Int64:
					fl = (*Int64SliceValue)(ptr)
				case reflect.Int:
					fl = (*IntSliceValue)(ptr)
				case reflect.Float32:
					fl = (*Float32SliceValue)(ptr)
				case reflect.Float64:
					fl = (*Float64SliceValue)(ptr)
				case reflect.String:
					fl = (*StringSliceValue)(ptr)
				case reflect.Bool:
					fl = (*BoolSliceValue)(ptr)
				default:
					return nil, fmt.Errorf("unrecognized slice element type: %v", elemTyp.String())
				}
			}
		case reflect.Array:
			elemTyp := typ.Elem()
			switch elemTyp.Kind() {
			case reflect.Uint8:
				expectedLen := val.Len()
				destSlice := val.Slice(0, expectedLen).Bytes()
				fl = &fixedLenBytes{
					Dest:           destSlice,
					ExpectedLength: uint64(expectedLen),
				}
			default:
				return nil, fmt.Errorf("unrecognized array element type: %v", elemTyp.String())
			}
		case reflect.Ptr:
			contentTyp := typ.Elem()
			// allocate a destination value if it doesn't exist yet
			if val.IsNil() {
				val.Set(reflect.New(contentTyp))
			}
			// and recurse into the type
			return FlagValue(typ.Elem(), val.Elem())
		default:
			return nil, fmt.Errorf("unrecognized type: %v", typ.String())
		}
	}
	return fl, nil
}
