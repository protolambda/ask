package ask

import (
	"context"
	"flag"
	"reflect"
)

type Command interface {
	Run(ctx context.Context) error
}

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

type Help interface {
	// Help explains how a command or group of flags is used.
	Help() string
}

var helpType = reflect.TypeOf((*Help)(nil)).Elem()

// InitDefault can be implemented by a command or flag
// to not rely on the user to prepare a default value,
// and instead move the responsibility to the command or flag itself.
// The default of a command is initialized before flags are applied.
// A command may embed multiple sub-structures that implement Default.
type InitDefault interface {
	// Default the flags of a command.
	Default()
}

var initDefaultType = reflect.TypeOf((*InitDefault)(nil)).Elem()

// Named is an optional extension of a Command,
// to name it for help/usage/debug information.
type Named interface {
	Command
	Name() string
}

type MoreHelp interface {
	// MoreHelp expands on the help information, e.g. by listing sub-commands.
	MoreHelp() string
}
