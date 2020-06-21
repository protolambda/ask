package ask

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/spf13/pflag"
	"net"
	"reflect"
	"strings"
	"time"
	"unsafe"
)

var UnrecognizedErr = errors.New("command was not recognized")

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

// Optionally specify how to get help information
// (usage of flags is added to this when called through CommandDescription.Usage() )
type Help interface {
	// Help explains how a command is used.
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

// An interface{} can be loaded as a command-description to execute it. See Load()
type CommandDescription struct {
	FlagsSet *pflag.FlagSet
	// Flags that can be passed as positional required args
	RequiredArgs []string
	// Flags that can be passed as positional optional args
	OptionalArgs []string
	// Command to run, may be nil if nothing has to run
	Command
	// Sub-command routing, can create commands (or other sub-commands) to access, may be nil if no sub-commands
	CommandRoute
	// Help Information as provided by the Help interface
	Help
}

// Load takes a structure instance that defines a command through its type,
// and the default values by determining them from the actual type.
func Load(val interface{}) (*CommandDescription, error) {
	return LoadReflect(reflect.ValueOf(val))
}

// LoadReflect is the same as Load, but directly using reflection to handle the value.
func LoadReflect(val reflect.Value) (*CommandDescription, error) {
	descr := &CommandDescription{
		FlagsSet: pflag.NewFlagSet("", pflag.ContinueOnError),
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
	if descr.Help == nil && typ.Implements(helpType) {
		descr.Help = val.Interface().(Help)
	}
	if typ.Implements(initDefaultType) {
		val.Interface().(InitDefault).Default()
	}
	switch val.Kind() {
	case reflect.Struct:
		fieldCount := val.NumField()
		for i := 0; i < fieldCount; i++ {
			f := typ.Field(i)
			tag, ok := getAsk(&f)
			// skip ignored fields
			if !ok || tag == "-" {
				continue
			}
			v := val.Field(i)
			// recurse into explicitly squashed fields
			if tag == "." {
				if err := descr.Load(v); err != nil {
					return err
				}
				continue
			}

			required, optional, err := descr.LoadField(typ.Field(i), v)
			if err != nil {
				return err
			}
			if required != "" {
				descr.RequiredArgs = append(descr.RequiredArgs, required)
			}
			if optional != "" {
				descr.OptionalArgs = append(descr.OptionalArgs, optional)
			}
		}
		return nil
	case reflect.Ptr:
		if val.IsNil() {
			val.Set(reflect.New(val.Type()))
		}
		return descr.LoadReflect(val.Elem())
	default:
		// Other types will be ignored.
		// E.g. you can have a function type as command, and just not load any flags.
		return nil
	}
}

// Usage prints the help information and the usage of all flags.
func (descr *CommandDescription) Usage() string {
	var out strings.Builder
	out.WriteString("(command)")
	if len(descr.RequiredArgs) > 0 {
		for _, a := range descr.RequiredArgs {
			out.WriteString(" <")
			out.WriteString(a)
			out.WriteString(">")
		}
	}
	if len(descr.OptionalArgs) > 0 {
		for _, a := range descr.OptionalArgs {
			out.WriteString(" [")
			out.WriteString(a)
			out.WriteString("]")
		}
	}
	out.WriteString("\n\n")
	if descr.FlagsSet.HasFlags() {
		out.WriteString("Flags/args:\n")
		out.WriteString(descr.FlagsSet.FlagUsages())
		out.WriteString("\n")
	}
	if descr.CommandRoute != nil {
		knownRoutes, ok := descr.CommandRoute.(CommandKnownRoutes)
		if ok {
			out.WriteString("Sub commands:\n")
			for _, k := range knownRoutes.Routes() {
				out.WriteString("  ")
				out.WriteString(k)
				if len(k) < 15 {
					out.WriteString(strings.Repeat(" ", 17-len(k)))
				} else {
					out.WriteString("  ")
				}
				subCmd, err := descr.CommandRoute.Cmd(k)
				if err != nil {
					out.WriteString("[error] failed to load command route")
				} else if subCmd == nil {
					out.WriteString("[error] command route not available")
				} else {
					subDescr, err := Load(subCmd)
					if err != nil {
						out.WriteString("[error] command is invalid")
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

// Runs the command, with given context and arguments.
// The final sub-command that actually runs is returned, and may be nil in case of an error.
// The "isHelp" will be true if help information was requested for the command (through `help`, `--help` or `-h`)
// To add inputs/outputs such as STDOUT to a command, they can be added as field in the command struct definition,
// and the command can pass them on to sub-commands. Similarly logging and other misc. data can be passed around.
// The execute parameters are kept minimal.
func (descr *CommandDescription) Execute(ctx context.Context, args ...string) (final *CommandDescription, isHelp bool, err error) {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		return descr, true, nil
	}

	if descr.CommandRoute != nil && len(args) > 0 {
		sub, err := descr.CommandRoute.Cmd(args[0])
		if err != nil {
			return nil, false, err
		}
		if sub != nil {
			subCmd, err := Load(sub)
			if err != nil {
				return nil, false, err
			}
			return subCmd.Execute(ctx, args[1:]...)
		}
		// deal with it as regular command if it is not recognized as sub-command
	}

	if err := descr.FlagsSet.Parse(args); err != nil && err != pflag.ErrHelp {
		return descr, false, err
	}
	var remainingPositionalRequiredFlags []string
	for _, v := range descr.RequiredArgs {
		if !descr.FlagsSet.Changed(v) {
			remainingPositionalRequiredFlags = append(remainingPositionalRequiredFlags, v)
		}
	}
	var remainingPositionalOptionalFlags []string
	for _, v := range descr.OptionalArgs {
		if !descr.FlagsSet.Changed(v) {
			remainingPositionalOptionalFlags = append(remainingPositionalOptionalFlags, v)
		}
	}

	// process required args
	remainingArgs := descr.FlagsSet.Args()
	if len(remainingArgs) < len(remainingPositionalRequiredFlags) {
		return descr, false, fmt.Errorf("got %d arguments, but expected %d, missing required arguments: %s",
			len(remainingArgs), len(remainingPositionalRequiredFlags), strings.Join(remainingPositionalRequiredFlags, ", "))
	}
	for i := range remainingPositionalRequiredFlags {
		if err := descr.FlagsSet.Set(remainingPositionalRequiredFlags[i], remainingArgs[i]); err != nil {
			return descr, false, err
		}
	}
	remainingArgs = remainingArgs[len(remainingPositionalRequiredFlags):]

	// process optional args
	if len(remainingPositionalOptionalFlags) > 0 {
		for i := range remainingArgs {
			if i >= len(remainingPositionalOptionalFlags) {
				break
			}
			if err := descr.FlagsSet.Set(remainingPositionalOptionalFlags[i], remainingArgs[i]); err != nil {
				return descr, false, err
			}
		}
		remainingArgs = remainingArgs[len(remainingPositionalOptionalFlags):]
	}

	if descr.Command != nil {
		err := descr.Command.Run(ctx, remainingArgs...)
		return descr, false, err
	}

	return descr, false, UnrecognizedErr
}

func getAsk(f *reflect.StructField) (v string, ok bool) {
	return f.Tag.Lookup("ask")
}

var pflagValueType = reflect.TypeOf((*pflag.Value)(nil)).Elem()

var durationType = reflect.TypeOf(time.Second)
var ipType = reflect.TypeOf(net.IP{})
var ipmaskType = reflect.TypeOf(net.IPMask{})
var ipNetType = reflect.TypeOf(net.IPNet{})

// Check the struct field, and add flag for it if asked for
func (descr *CommandDescription) LoadField(f reflect.StructField, val reflect.Value) (requiredArg, optionalArg string, err error) {
	if !val.CanAddr() {
		return
	}
	v, ok := getAsk(&f)
	if !ok {
		return
	}
	name := ""
	shorthand := ""
	deprecated := ""
	help := ""
	hidden := false

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

	for _, k := range strings.Split(v, " ") {
		if k == "" {
			continue
		}
		if strings.HasPrefix(k, "--") {
			name = k[2:]
			continue
		}

		if strings.HasPrefix(k, "-") {
			shorthand = k[1:]
			continue
		}
	}

	if strings.HasPrefix(v, "<") && strings.HasSuffix(v, ">") {
		name = v[1 : len(v)-1]
		requiredArg = name
	}
	if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
		name = v[1 : len(v)-1]
		optionalArg = name
	}

	// Declare that the field can be parsed
	ok = true

	flags := descr.FlagsSet

	if err := AddFlag(flags, f.Type, val, name, shorthand, help); err != nil {
		return "", "", err
	}

	if deprecated != "" {
		_ = flags.MarkDeprecated(name, deprecated)
	}
	if hidden {
		_ = flags.MarkHidden(name)
	}
	return
}

func AddFlag(flags *pflag.FlagSet, typ reflect.Type, val reflect.Value, name string, shorthand string, help string) error {
	// Get the pointer to the destination struct, to route pflags to
	ptr := unsafe.Pointer(val.Addr().Pointer())

	// Create the right pflag based on the type
	if typ.Implements(pflagValueType) {
		pVal := val.Interface().(pflag.Value)
		flags.AddFlag(&pflag.Flag{
			Name:      name,
			Shorthand: shorthand,
			Usage:     help,
			Value:     pVal,
			DefValue:  pVal.String(),
		})
	} else if reflect.PtrTo(typ).Implements(pflagValueType) {
		pVal := val.Addr().Interface().(pflag.Value)
		flags.AddFlag(&pflag.Flag{
			Name:      name,
			Shorthand: shorthand,
			Usage:     help,
			Value:     pVal,
			DefValue:  pVal.String(),
		})
	} else if typ == durationType {
		flags.DurationVarP((*time.Duration)(ptr), name, shorthand, time.Duration(val.Int()), help)
	} else if typ == ipType {
		flags.IPVarP((*net.IP)(ptr), name, shorthand, net.IP(val.Bytes()), help)
	} else if typ == ipNetType {
		flags.IPNetVarP((*net.IPNet)(ptr), name, shorthand, val.Interface().(net.IPNet), help)
	} else if typ == ipmaskType {
		flags.IPMaskVarP((*net.IPMask)(ptr), name, shorthand, val.Interface().(net.IPMask), help)
	} else {
		switch typ.Kind() {
		// unsigned integers
		case reflect.Uint:
			flags.UintVarP((*uint)(ptr), name, shorthand, uint(val.Uint()), help)
		case reflect.Uint8:
			flags.Uint8VarP((*uint8)(ptr), name, shorthand, uint8(val.Uint()), help)
		case reflect.Uint16:
			flags.Uint16VarP((*uint16)(ptr), name, shorthand, uint16(val.Uint()), help)
		case reflect.Uint32:
			flags.Uint32VarP((*uint32)(ptr), name, shorthand, uint32(val.Uint()), help)
		case reflect.Uint64:
			flags.Uint64VarP((*uint64)(ptr), name, shorthand, val.Uint(), help)
		// signed integers
		case reflect.Int:
			flags.IntVarP((*int)(ptr), name, shorthand, int(val.Int()), help)
		case reflect.Int8:
			flags.Int8VarP((*int8)(ptr), name, shorthand, int8(val.Int()), help)
		case reflect.Int16:
			flags.Int16VarP((*int16)(ptr), name, shorthand, int16(val.Int()), help)
		case reflect.Int32:
			flags.Int32VarP((*int32)(ptr), name, shorthand, int32(val.Int()), help)
		case reflect.Int64:
			flags.Int64VarP((*int64)(ptr), name, shorthand, val.Int(), help)
		// Misc
		case reflect.String:
			flags.StringVarP((*string)(ptr), name, shorthand, val.String(), help)
		case reflect.Bool:
			flags.BoolVarP((*bool)(ptr), name, shorthand, val.Bool(), help)
		case reflect.Float32:
			flags.Float32VarP((*float32)(ptr), name, shorthand, float32(val.Float()), help)
		case reflect.Float64:
			flags.Float64VarP((*float64)(ptr), name, shorthand, val.Float(), help)
		// Cobra commons
		case reflect.Slice:
			elemTyp := typ.Elem()
			if elemTyp == durationType {
				data := (*[]time.Duration)(ptr)
				flags.DurationSliceVarP(data, name, shorthand, *data, help)
			} else if elemTyp == ipType {
				data := (*[]net.IP)(ptr)
				flags.IPSliceVarP(data, name, shorthand, *data, help)
			} else {
				switch elemTyp.Kind() {
				case reflect.Uint8:
					b := (*[]byte)(ptr)
					pVal := (*BytesHexFlag)(b)
					flags.AddFlag(&pflag.Flag{
						Name:      name,
						Shorthand: shorthand,
						Usage:     help,
						Value:     pVal,
						DefValue:  pVal.String(),
					})
				case reflect.Uint:
					data := (*[]uint)(ptr)
					flags.UintSliceVarP(data, name, shorthand, *data, help)
				case reflect.Int:
					data := (*[]int)(ptr)
					flags.IntSliceVarP(data, name, shorthand, *data, help)
				case reflect.Int32:
					data := (*[]int32)(ptr)
					flags.Int32SliceVarP(data, name, shorthand, *data, help)
				case reflect.Int64:
					data := (*[]int64)(ptr)
					flags.Int64SliceVarP(data, name, shorthand, *data, help)
				case reflect.Float32:
					data := (*[]float32)(ptr)
					flags.Float32SliceVarP(data, name, shorthand, *data, help)
				case reflect.Float64:
					data := (*[]float64)(ptr)
					flags.Float64SliceVarP(data, name, shorthand, *data, help)
				case reflect.String:
					data := (*[]string)(ptr)
					flags.StringSliceVarP(data, name, shorthand, *data, help)
				case reflect.Bool:
					data := (*[]bool)(ptr)
					flags.BoolSliceVarP(data, name, shorthand, *data, help)
				default:
					return fmt.Errorf("unrecognized slice element type: %v", elemTyp.String())
				}
			}
		case reflect.Array:
			elemTyp := typ.Elem()
			switch elemTyp.Kind() {
			case reflect.Uint8:
				expectedLen := val.Len()
				destSlice := val.Slice(0, expectedLen).Bytes()
				pVal := &fixedLenBytes{
					Dest:           destSlice,
					ExpectedLength: uint64(expectedLen),
				}
				flags.AddFlag(&pflag.Flag{
					Name:      name,
					Shorthand: shorthand,
					Usage:     help,
					Value:     pVal,
					DefValue:  pVal.String(),
				})
			default:
				return fmt.Errorf("unrecognized array element type: %v", elemTyp.String())
			}
		case reflect.Ptr:
			contentTyp := typ.Elem()
			// allocate a destination value if it doesn't exist yet
			if val.IsNil() {
				val.Set(reflect.New(contentTyp))
			}
			// and recurse into the type
			return AddFlag(flags, typ.Elem(), val.Elem(), name, shorthand, help)
		default:
			// TODO: more flag types?
			return fmt.Errorf("unrecognized type: %v", typ.String())
		}
	}
	return nil
}

// BytesHex exposes bytes as a flag, hex-encoded,
// optional whitespace padding, case insensitive, and optional 0x prefix.
type BytesHexFlag []byte

func (f BytesHexFlag) String() string {
	return hex.EncodeToString(f)
}

func (f *BytesHexFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	if strings.HasPrefix(value, "0x") {
		value = value[2:]
	}
	b, err := hex.DecodeString(value)
	if err != nil {
		return err
	}
	*f = b
	return nil
}

func (f *BytesHexFlag) Type() string {
	return "bytes"
}

// fixedLenBytes exposes fixed-length bytes as a flag, hex-encoded,
// optional whitespace padding, case insensitive, and optional 0x prefix.
type fixedLenBytes struct {
	Dest           []byte
	ExpectedLength uint64
}

func (f fixedLenBytes) String() string {
	return hex.EncodeToString(f.Dest)
}

func (f *fixedLenBytes) Set(value string) error {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	if strings.HasPrefix(value, "0x") {
		value = value[2:]
	}
	b, err := hex.DecodeString(value)
	if err != nil {
		return err
	}
	if uint64(len(b)) != f.ExpectedLength {
		return fmt.Errorf("byte length does not match fixed-length of %d bytes, "+
			"parsed %d bytes", f.ExpectedLength, len(b))
	}
	copy(f.Dest, b)
	return nil
}

func (f *fixedLenBytes) Type() string {
	return fmt.Sprintf("bytes%d", f.ExpectedLength)
}
