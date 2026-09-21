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

// InitDefault can be implemented by a command, flag group or flag value
// to not rely on the user to prepare a default value,
// and instead move the responsibility to the command, group or flag itself.
//
// Defaults are applied exactly once per implementation, before any flag is loaded
// from env vars or args, and bottom-up: first the flag values of a struct,
// then the struct itself, then the struct embedding it, and so on.
// Thus a command has the final say over the defaults of its embedded groups.
// If a Default replaces a pointer to a group or flag value, the replacement is used as-is:
// its own Default is not applied.
//
// Default overwrites whatever values the command was pre-configured with,
// unless the implementation checks for existing values.
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
