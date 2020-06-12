package ask

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

func TestPeerConnect(t *testing.T) {
	state := ActorState{
		HostData: "old value",
	}
	defaultPeer := Peer{State: &state}
	cmd, err := Load(&defaultPeer)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := cmd.Execute(context.Background(), "bad"); err != NotRecognizedErr {
		t.Fatal(err)
	}

	if cmd, isHelp, err := cmd.Execute(context.Background(), "connect", "--help"); err != nil {
		t.Fatal(err)
	} else if !isHelp {
		t.Fatal("expected help")
	} else {
		usage := cmd.Usage("connect")
		if !strings.HasPrefix(usage, "connect [flags...] <data> <id> [more]") {
			t.Fatal("expected usage string starting with command usage info")
		}
		if !strings.Contains(usage, "Flags/args") {
			t.Fatal("expected usage string with flags information")
		}
	}

	// Execute returns the final command that is executed,
	// to get the subcommands in case usage needs to be printed, or other result data is required.
	if cmd, isHelp, err := cmd.Execute(context.Background(),
		strings.Split("connect --addr 1.2.3.4 --port=4000 --tag=123hey 42 someid optionalhere", " ")...); err != nil {
		t.Fatal(err)
	} else if isHelp {
		// print usage if the user asks --help
		t.Log(cmd.Usage("connect"))
	}

	if state.HostData != "1.2.3.4:4000 #123hey $42 someid ~ optionalhere" {
		t.Errorf("got unexpected host data value: %s", state.HostData)
	}
}
