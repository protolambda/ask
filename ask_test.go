package ask

import (
	"context"
	"encoding/hex"
	"errors"
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

type PeerOptions struct {
	Tag    string `ask:"--tag" help:"tag to give to peer"`
	PeerID string `ask:"<id>" help:"libp2p ID of the peer, if no address is specified, the peer is looked up in the peerstore"`
}

type MiscOptions struct {
	Data    uint8 `ask:"<data>" help:"some number"`
	Awesome bool  `ask:"--awesome" help:"Enable awesome feature"`
	Bad     bool  `ask:"--bad" help:"Enable bad feature"`
}

type InlineOptions struct {
	Foobar []int32 `ask:"--foobar" help:"foobar integers"`
	Bytes  []byte  `ask:"--hex" help:"Hex value"`
}

func (opts *InlineOptions) Default() {
	opts.Foobar = []int32{4, 5, 6}
}

type Connect struct {
	*ActorState
	Addr          net.IP `ask:"--addr" help:"address to connect to"`
	Port          uint16 `ask:"--port" help:"port to use for connection"`
	PeerOptions   `ask:".peer" help:"Options for peer stuff"`
	MiscOptions   `ask:".misc" help:"Misc. options"`
	InlineOptions `ask:"."`
	ForkOptions   struct {
		Digests [][3]byte `ask:"--digests" help:"some digests"`
		More    string    `ask:"[more]" help:"something optional"`
	} `ask:".fork" help:"Fork options"`
	PortIsSet bool `changed:"port"`
	AddrIsSet bool `changed:"addr"`
}

func (c *Connect) Help() string {
	return "Connect to a peer"
}

func (c *Connect) Default() {
	c.Port = 9000
	c.ForkOptions.Digests = [][3]byte{{0xa1, 0xb2, 0xc3}, {0xd4, 0xe5, 0xf6}}
	c.Bad = true
}

func (c *Connect) Run(ctx context.Context, args ...string) error {
	digests := ""
	for _, d := range c.ForkOptions.Digests {
		digests += hex.EncodeToString(d[:]) + "!"
	}
	if c.PortIsSet {
		return errors.New("expected port not to be set explicitly")
	}
	if !c.AddrIsSet {
		return errors.New("expected addr to be set explicitly")
	}
	c.HostData = fmt.Sprintf("%s:%d #%s $%d %s ~ %s, remaining: %s ~ digests: %s ~ awesome: %v, bad: %v ~ foobar: %v ~ hex: %x",
		c.Addr.String(), c.Port, c.Tag, c.Data, c.PeerID, c.ForkOptions.More, strings.Join(args, ", "), digests, c.Awesome, c.Bad, c.Foobar, c.Bytes)
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

	if _, err := cmd.Execute(context.Background(), nil, "bad"); err != UnrecognizedErr {
		t.Fatal(err)
	}

	usage := cmd.Usage(false)
	if !strings.HasPrefix(usage, "(command)\n\nSub commands:\n") {
		t.Fatal("expected usage string starting with sub command header info")
	}
	if !strings.Contains(usage, "Connect to a peer") {
		t.Fatal("expected usage string with connect sub command")
	}

	if cmd, err := cmd.Execute(context.Background(), nil, "connect", "--help"); err != nil && err != HelpErr {
		t.Fatal(err)
	} else if err != HelpErr {
		t.Fatal("expected help")
	} else {
		usage := cmd.Usage(false)
		if !strings.HasPrefix(usage, "(command) <peer.id> <misc.data> [fork.more]") {
			t.Fatalf("expected usage string starting with command usage info, got: %s", usage)
		}
		if !strings.Contains(usage, "9000") {
			t.Fatalf("expected default to be included in help details, got: %s", usage)
		}
		if !strings.Contains(usage, "a1b2c3,d4e5f6") {
			t.Fatalf("expected default digest value to be included, got: %s", usage)
		}
		if !strings.Contains(usage, "default: 4,5,6") {
			t.Fatalf("expected embedded default to be included in help details, got: %s", usage)
		}
	}

	// Execute returns the final command that is executed,
	// to get the subcommands in case usage needs to be printed, or other result data is required.
	if _, err := cmd.Execute(context.Background(), nil,
		strings.Split("connect --addr 1.2.3.4 --peer.tag=123hey somepeerid 42 optionalhere --misc.bad=false --misc.awesome --fork.digests=a1b2c3,42e5f6,a1b2c3 --foobar=2,0x123,-1,8 --hex 0x1234567890 extra more", " ")...); err != nil {
		t.Fatal(err)
	}

	if state.HostData != "1.2.3.4:9000 #123hey $42 somepeerid ~ optionalhere, remaining: extra, more ~ digests: a1b2c3!42e5f6!a1b2c3! ~ awesome: true, bad: false ~ foobar: [2 291 -1 8] ~ hex: 1234567890" {
		t.Errorf("got unexpected host data value: %s", state.HostData)
	}
}
