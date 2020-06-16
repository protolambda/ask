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
	*ActorState
}

func (c *Peer) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "connect":
		return &Connect{ActorState: c.ActorState}, nil
	default:
		return nil, UnrecognizedErr
	}
}

func (c *Peer) Routes() []string {
	return []string{"connect"}
}

type Connect struct {
	*ActorState
	Addr   net.IP `ask:"--addr" help:"address to connect to"`
	Port   uint16 `ask:"--port" help:"port to use for connection"`
	Tag    string `ask:"--tag" help:"tag to give to peer"`
	Data   uint8  `ask:"<data>" help:"some number"`
	PeerID string `ask:"<id>" help:"libp2p ID of the peer, if no address is specified, the peer is looked up in the peerstore"`
	More   string `ask:"[more]" help:"optional"`
}

func (c *Connect) Help() string {
	return "connect to a peer"
}

func (c *Connect) Default() {
	c.Port = 9000
}

func (c *Connect) Run(ctx context.Context, args ...string) error {
	c.HostData = fmt.Sprintf("%s:%d #%s $%d %s ~ %s, remaining: %s",
		c.Addr.String(), c.Port, c.Tag, c.Data, c.PeerID, c.More, strings.Join(args, ", "))
	return nil
}

func TestPeerConnect(t *testing.T) {
	state := ActorState{
		HostData: "old value",
	}
	defaultPeer := Peer{ActorState: &state}
	cmd, err := Load(&defaultPeer)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := cmd.Execute(context.Background(), "bad"); err != UnrecognizedErr {
		t.Fatal(err)
	}

	usage := cmd.Usage("peer")
	if !strings.HasPrefix(usage, "peer\n\nSub commands:\n") {
		t.Fatal("expected usage string starting with sub command header info")
	}
	if !strings.Contains(usage, "connect to a peer") {
		t.Fatal("expected usage string with connect sub command")
	}

	if cmd, isHelp, err := cmd.Execute(context.Background(), "connect", "--help"); err != nil {
		t.Fatal(err)
	} else if !isHelp {
		t.Fatal("expected help")
	} else {
		usage := cmd.Usage("connect")
		if !strings.HasPrefix(usage, "connect <data> <id> [more]") {
			t.Fatal("expected usage string starting with command usage info")
		}
		if !strings.Contains(usage, "Flags/args") {
			t.Fatal("expected usage string with flags information")
		}
		if !strings.Contains(usage, "9000") {
			t.Fatal("expected default to be included in help details")
		}
	}

	// Execute returns the final command that is executed,
	// to get the subcommands in case usage needs to be printed, or other result data is required.
	if _, _, err := cmd.Execute(context.Background(),
		strings.Split("connect --addr 1.2.3.4 --tag=123hey 42 someid optionalhere extra more", " ")...); err != nil {
		t.Fatal(err)
	}

	if state.HostData != "1.2.3.4:9000 #123hey $42 someid ~ optionalhere, remaining: extra, more" {
		t.Errorf("got unexpected host data value: %s", state.HostData)
	}
}
