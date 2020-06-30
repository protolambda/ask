# Ask

Ask is a small CLI building package for Go, which enables you to define commands as datat-types, without requiring full initialization upfront.
This makes it suitable for shell applications, and CLIs with dynamic commands or just too many to load at once.

It has minimal dependencies: only [`github.com/spf13/pflag`](https://github.com/spf13/pflag)
 for excellent and familiar flag parsing (Cobra CLI is powered with these flags).

In addition, some special array/slice types are supported:
- `[]byte` as hex-encoded string, case-insensitive, optional `0x` prefix and padding
- `[N]byte`, same as above, but an array
- `[][N]byte`, a comma-separated list of elements, each formatted like the above.

Warning: this is a new experimental package, built to improve the [`Rumor`](https://github.com/protolambda/rumor) shell.

Note: flags in between command parts, e.g. `peer --foobar connect ` are not supported, but may be in the future.

## Usage

```go
// load a command struct
cmd, err := Load(MyCommandStruct{})

// Execute a command
cmd, isHelp, err := cmd.Execute(context.Background(), "some", "args", "--here", "use", "a", "shell", "parser")
```

Thanks to `pflag`, all basic types, slices (well, work in progress), and some misc network types are supported for flags.

You can also implement the `pflag.Value` interface for custom flag parsing. 

To define a command, implement `Command` and/or `CommandRoute`:

`func (c *Command) Run(ctx context.Context, args ...string) error { ... }`

`Cmd(route string) (cmd interface{}, err error)`

To hint at sub-command routes, implement `CommandKnownRoutes`:

`Routes() []string`

For additional help information, a command can also implement `Help`:

`func (c Connect) Help() string { ... }`

The help information, along usage info (flag set info + default values + sub commands list) can 
be retrieved from `.Usage()` after `Load()`-ing the command.

For default options that are not `""` or `0` or other Go defaults, the `Default()` interface can be implemented on a command, 
to set its flag values during `Load()`. 

## Example

```go
package main

import (
    "context"
    "fmt"
    "github.com/protolambda/ask"
    "net"
    "strings"
    "testing"
)

type ActorState struct {
    HostData string
}

type Peer struct {
    *ActorState
}

func (c *Peer) Cmd(route string) (cmd interface{}, err error) {
    switch route {
    case "connect":
        return &Connect{ActorState: c.ActorState}, nil
    default:
        return nil, ask.UnrecognizedErr
    }
}

func (c *Peer) Routes() []string {
    return []string{"connect"}
}

type Connect struct {
    // Do not embed the parent command, or Connect will be recognized as a command route.
    // If recursive commands are desired, the command route can return a nil command
    // if the command itself should be evaluated as a normal command instead.
    *ActorState
    Addr   net.IP `ask:"--addr" help:"address to connect to"`
    Port   uint16 `ask:"--port" help:"port to use for connection"`
    Tag    string `ask:"--tag" help:"tag to give to peer"`
    Data   uint8  `ask:"<data>" help:"some number"`
    PeerID string `ask:"<id>" help:"libp2p ID of the peer, if no address is specified, the peer is looked up in the peerstore"`
    More   string `ask:"[more]" help:"optional"`
}

func (c *Connect) Default() {
    c.Port = 9000
}

func (c *Connect) Help() string {
    return "connect to a peer"
}

func (c *Connect) Run(ctx context.Context, args ...string) error {
    c.HostData = fmt.Sprintf("%s:%d #%s $%d %s ~ %s, remaining: %s",
        c.Addr.String(), c.Port, c.Tag, c.Data, c.PeerID, c.More, strings.Join(args, ", "))
    return nil
}

func main() {
    state := ActorState{
        HostData: "old value",
    }
    defaultPeer := Peer{ActorState: &state}
    cmd, err := ask.Load(&defaultPeer)
    if err != nil {
        t.Fatal(err)
    }

    // Execute returns the final command that is executed,
    // to get the subcommands in case usage needs to be printed, or other result data is required.
    cmd, isHelp, err := cmd.Execute(context.Background(),
        strings.Split("connect --addr 1.2.3.4 --tag=123hey 42 someid optionalhere extra more", " ")...)
    // handle err
    if err == nil {
        panic(err)
    }
    if isHelp {
        // print usage if the user asks --help
        fmt.Println(cmd.Usage())
    }

    // use resulting state change
    // state.HostData == "1.2.3.4:4000 #123hey $42 someid ~ optionalhere, remaining: extra, more"
}
```


## License

MIT, see [`LICENSE`](./LICENSE) file.
