# Ask

Ask is a small CLI building package for Go.

Commands are defined as data-types,
and sub-commands do not require initialization upfront.
This makes it suitable for shell applications,
and CLIs with dynamic commands or just too many to load at once. 

Ask is composable and open, it is designed for highly re-usable flags,
flag-groups and command extensibility.

In addition to common Go basic types, some special array/slice types are supported:
- `[](u)int(8/16/32/64)`: integer slices
- `[]string`: string slices (with CSV-like delimiter decoding, thanks pflag for the idea)
- `net.IP`, `net.IPMask`, `net.IPNet`: common networking flags
- `[]byte` as hex-encoded string, case-insensitive, optional `0x` prefix and padding
- `[N]byte`, same as above, but an array
- `[][N]byte`, a comma-separated list of elements, each formatted like the above.

## Flags

Each flag and (optional) argument is declared as a struct-field in a command.
Commands can be composed of different structs, inlined or grouped.
Ask is designed to make command options as reusable as possible.

Struct tags:
- `ask`: to declare a field as flag/arg.
  - `ask:"<mainthing>"`: a positional required argument (alias: `--mainthing`)
  - `ask:"[extrathing]`: a positional optional argument (alias: `--extrathing`)
  - `ask:"--my-flag`: a long flag
  - `ask:"-v`: a shorthand flag
  - `ask:"--verbose -v"`: a long flag with shorthand
  - `ask:".`: inline group
  - `ask:".groupnamehere`: flag group (can be nested)
- `help:"Infomation about flag here"`: define flag / flag-group usage info
- `hidden:"any value"`: to hide a flag from usage info
- `deprecated:"reason here"`: to mark a flag as deprecated
- environment variable support:
  - `ask:"--my-flag"` automatically becomes `MY_FLAG`
  - `env:"SPECIAL_ENV_NAME"`: to load as `SPECIAL_ENV_NAME`
  - `env:"-"`: disables env, flag/arg only.

Example:
```go
type BoundCmd struct {
    LowBound uint64 `ask:"--low -l" help:"Lower bound"`
    HighBound uint64 `ask:"--high -h" help:"higher bound"`
    KeyParam string `ask:"<key-param>" help:"Parameter to bound check"`
}
```

### Inline group flags

Use `ask:"."` to mark the field as an inline group. The field can be regular or embedded.
The below is equivalent to the above `BoundCmd`.

```go
type Bounds struct {
    LowBound uint64 `ask:"--low -l" help:"Lower bound"`
    HighBound uint64 `ask:"--high -h" help:"higher bound"`
}

type BoundCmd struct {
	Bounds `ask:"."`
    KeyParam string `ask:"<key-param>" help:"Parameter to bound check"`
}
```

### Group flags

Grouping flags helps avoid naming collisions, organizes the flags, and enables group-wise documentation.
Groups start with `.` in the ask field declaration:

```go
type ConnOptions struct {
	Port uint16 `ask:"--port"`
	IP   net.IP `ask:"--ip"`
}

type NodeCmd struct {
	Websocket   ConnOptions `ask:".ws" help:"Websocket connection options"`
    Tcp         ConnOptions `ask:".tcp" help:"Websocket connection options"`
}
```

And then flags look like:
```
my-node-cmd --ws.port=5000 --ws.ip=1.2.3.4 --tcp.port=8080 --tcp.ip=5.6.7.8
```

## Running commands

Implement the `Command` interface to make a command executable:
```go
func (c *BoundCmd) Run(ctx context.Context) error {
	val := getExternalValue(ctx, c.KeyParam)
	if val < c.LowBound || val > c.HighBound {
		return fmt.Errorf("val %d (%s) out of bounds %d <> %d", val, c.KeyParam, c.LowBound, c.HighBound)
    }
    return nil
}
```

The struct flags/args will be fully initialized before `Run` executes.
Any unparsed trailing arguments can be accessed through `ask.Args(ctx)`.

### `InitDefault`

Commands can implement the `InitDefault` interface to specify non-zero flag defaults.

```go
func (c *BoundCmd) Default() {
	c.LowBound = 20
	c.HighBound = 45
}
```

### Routing sub-commands

Sub-commands are simply nested Run calls.

```go
func (c *OuterCmd) Run(ctx context.Context) error {
    subCmd, subArgs := SplitArgs(ctx)
    switch subCmd {
    case "foo":
        return Run(ctx, &FooCmd{}, subArgs)
    case "bar":
        return Run(ctx, &BarCmd{}, subArgs)
    default:
        return UnrecognizedErr
    }
}
```

Commands can pre-configure inner commands, dynamically route, or even recurse.

## `Help`

- Commands and flag groups can implement the `Help() string` interface to output (dynamic) usage information.
- Flag-groups and flags can specify the `help` struct-tag to declare static usage information.

```go
func (c *BoundCmd) Help() string {
	return fmt.Sprintf("checks if the %s is within bounds", c.KeyParam)
}
```

### Sub-command listing

Since routing is dynamic, the command usage information doesn't print sub-commands by default.
To describe sub-commands, implement `MoreHelp`.
The `SubCommandsUsage` can generate a simple sub-commands overview:
```go
func (c *OuterCmd) MoreHelp() string {
	return SubCommandsUsage(&FooCmd{}, &BarCmd)
}
```

### Output command usage

When encountering `-h` or `--help`, `Run` returns a `ask.HelpErr`,
wrapped with information about the command.
When wrapping sub-command errors, the inner-most command info is preserved.

The usage of the failed command (upon `ask.HelpErr` or any other `Run` error)
can be retrieved with `UsageFromErr`:

```go
fmt.Println(UsageFromErr(err))
```

## Flag customization

### `flag.Value`

The standard Go flag `Value` interface `func String() string, func Set(string) error` can be used to define custom flags.

```go
// ENRFlag wraps an ENR (special encoded address string) to make it a reusable flag type
type ENRFlag enr.Record

func (f *ENRFlag) String() string {
	enrStr, err := addrutil.EnrToString((*enr.Record)(f))
	if err != nil {
		return "? (invalid ENR)"
	}
	return enrStr
}

func (f *ENRFlag) Set(v string) error {
	enrAddr, err := addrutil.ParseEnr(v)
	if err != nil {
		return err
	}
	*f = *(*ENRFlag)(enrAddr)
	return nil
}
```

### `TypedValue`

A custom flag type can be explicit about its type to enhance usage information, and not rely on a help description for repetitive type information.

```go
func (f *ENRFlag) Type() string {
    return "ENR"
}
```

### `ImplicitValue`

A boolean flag can omit the value to be interpreted as True, e.g. `my-cli do something --awesome`.
For a flag to have an implicit value, implement this interface.

```go
func (b *BoolValue) Implicit() string {
	return "true"
}
```

## License

MIT, see [`LICENSE`](./LICENSE) file.
