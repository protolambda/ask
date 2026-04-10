// Package ask implements ask command handling
//
// Structs are parsed as a command.
// Struct fields are used to annotate flags:
// - `ask:"--example,-e"` to define a flag name and/or shorthand alias
// - `help:"This is an example flag"` to define info on how to use the flag
// - `ask:"-"` to ignore the field / embedded struct
// - `ask:"."` to handle an embedded struct as flags, instead of a single flag value.
// - `ask:".example" on the embedded struct to prefix every field, like a flag group.
// - `hidden:""` to hide a flag
// - `deprecated:"reason here"` to deprecate a flag, with deprecation reason
// - `env:"EXAMPLE_FLAG"` to load it as an env var (flag nesting does not change manually defined env vars).
// - `env:"-"` to not default to translating "--thing.example" to "THING_EXAMPLE` as env var alternative.
//
// To confirm a flag is set, see IsSet.
//
// Commands can implement the following interfaces:
// - InitDefault
// - Help
// - MoreHelp to list sub-commands, tips etc.
// - Command
//
// Embedded structs (flag groups) can implement the following interfaces:
// - InitDefault
// - Help (overridden by help struct tag on embedding site, if any)
//
// Flags can implement the following interfaces:
// - InitDefault
// - TypedValue
// - ImplicitValue
//
// Commands are executed with Run.
//
// A running command can start sub-command(s) by calling Run within the same context.
// When `--help` is passed as arg, the actual command handler does not Run,
// and a HelpErr is returned upon Run instead.
// A Run error can be unpacked with DescriptionFromErr to get the failed command,
// to print usage info or debug it.
package ask
