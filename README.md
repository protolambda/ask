# Ask

Ask is a small CLI building package for Go, which enables you to define commands as datat-types, without requiring full initialization upfront.
This makes it suitable for shell applications, and CLIs with dynamic commands or just too many to load at once.

It has minimal dependencies: only [`github.com/spf13/pflag`](https://github.com/spf13/pflag)
 for excellent and familiar flag parsing (Cobra CLI is powered with these flags).

Warning: this is a new experimental package, built to improve the [`Rumor`](https://github.com/protolambda/rumor) shell.


## Usage

```go
// load a command struct
cmd, err := Load(MyCommandStruct{})

// Execute a command
cmd, isHelp, err := cmd.Execute(context.Background(), "some", "args", "--here", "use", "a", "shell", "parser")
```

Thanks to `pflag`, all basic types, slices (well, work in progress), and some misc network types are supported for flags.

You can also implement the `pflag.Value` interface for custom flag parsing. 

To define a command, implement `Command` and/or `RouteCommand`:

`func (c *Command) Run(ctx context.Context, args ...string) error { ... }`

`func (c *MyHubCommand) Get(ctx context.Context, args ...string) (cmd interface{}, remaining []string, err error) { ... }`

For additional help information, a command can also implement `Help`:

`func (c Connect) Help() string { ... }`


## Example

```go
package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
)

type ActorState struct {
	HostData string
}

type Peer struct {
	State *ActorState
}

func (c *Peer) Get(ctx context.Context, args ...string) (cmd interface{}, remaining []string, err error) {
	switch args[0] {
	case "connect":
		return &Connect{Parent: c, State: c.State}, args[1:], nil
	default:
		return nil, args, NotRecognizedErr
	}
}

type Connect struct {
	Parent *Peer
	State  *ActorState
	Addr   net.IP `ask:"--addr" help:"address to connect to"`
	Port   uint16 `ask:"--port" help:"port to use for connection"`
	Tag    string `ask:"--tag" help:"tag to give to peer"`
	Data   uint8  `ask:"<data>" help:"some number"`
	PeerID string `ask:"<id>" help:"libp2p ID of the peer, if no address is specified, the peer is looked up in the peerstore"`
	More   string `ask:"[more]" help:"optional"`
}

func (c Connect) Help() string {
	return "connect to a peer"
}

func (c *Connect) Run(ctx context.Context, args ...string) error {
	c.State.HostData = fmt.Sprintf("addr: %s:%d #%s $%d %s ~ %s, remaining: %s",
		c.Addr.String(), c.Port, c.Tag, c.Data, c.PeerID, c.More, strings.Join(args, ", "))
	return nil
}

func main() {
    state := ActorState{
		HostData: "old value",
	}
	defaultPeer := Peer{State: &state}
	cmd, err := Load(&defaultPeer)
	if err != nil {
		t.Fatal(err)
	}

	// Execute returns the final command that is executed,
	// to get the subcommands in case usage needs to be printed, or other result data is required.
    cmd, isHelp, err := cmd.Execute(context.Background(),
	    strings.Split("connect --addr 1.2.3.4 --port=4000 --tag=123hey 42 someid optionalhere extra more", " ")...)
    // handle err
    if err == nil {
        panic(err)
    }
	if isHelp {
		// print usage if the user asks --help
		fmt.Println(cmd.Usage("connect"))
	}

    // use resulting state change
	// state.HostData == "1.2.3.4:4000 #123hey $42 someid ~ optionalhere, remaining: extra, more"
}
```


## License

MIT, see [`LICENSE`](./LICENSE) file.
