package ask

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
)

type ActorState struct {
	HostData string
}

type Peer struct {
	*ActorState
}

func (c *Peer) Run(ctx context.Context) error {
	subCmd, subArgs := SplitArgs(ctx)
	switch subCmd {
	case "connect":
		return Run(ctx, &Connect{ActorState: c.ActorState}, subArgs)
	default:
		return UnrecognizedErr
	}
}

func (c *Peer) MoreHelp() string {
	return SubCommandsUsage(&Connect{})
}

func (c *Peer) Name() string {
	return "peer"
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
	*ActorState // state can be passed on easily from the parent command

	Addr          net.IP `ask:"--addr" help:"address to connect to"`
	Port          uint16 `ask:"--port" help:"port to use for connection"`
	PeerOptions   `ask:".peer" help:"Options for peer stuff"`
	MiscOptions   `ask:".misc" help:"Misc. options"`
	InlineOptions `ask:"."`
	ForkOptions   struct {
		Digests [][3]byte `ask:"--digests" help:"some digests"`
		More    string    `ask:"[more]" help:"something optional"`
	} `ask:".fork" help:"Fork options"`
}

func (c *Connect) Name() string {
	return "connect"
}

func (c *Connect) Help() string {
	return "Connect to a peer"
}

func (c *Connect) Default() {
	c.Port = 9000
	c.ForkOptions.Digests = [][3]byte{{0xa1, 0xb2, 0xc3}, {0xd4, 0xe5, 0xf6}}
	c.Bad = true
}

func (c *Connect) Run(ctx context.Context) error {
	digests := ""
	for _, d := range c.ForkOptions.Digests {
		digests += hex.EncodeToString(d[:]) + "!"
	}
	portIsSet := IsSet(ctx, "port")
	addrIsSet := IsSet(ctx, "addr")
	if portIsSet {
		return errors.New("expected port not to be set explicitly")
	}
	if !addrIsSet {
		return errors.New("expected addr to be set explicitly")
	}
	args := Args(ctx)
	fmt.Println("addr:", c.Addr)
	fmt.Println("port:", c.Port)
	fmt.Println("tag:", c.Tag)
	fmt.Println("data:", c.Data)
	fmt.Println("fork opts more:", c.ForkOptions.More)
	fmt.Printf("remaining args: %v\n", args)
	fmt.Println("digests:", digests)
	fmt.Println("awesome:", c.Awesome)
	fmt.Println("bad:", c.Bad)
	fmt.Println("foobar:", c.Foobar)
	fmt.Println("bytes:", c.Bytes)
	c.HostData = "done!"
	return nil
}

func ExampleRun_peer_connect() {
	state := ActorState{
		HostData: "old value",
	}
	cmd := &Peer{ActorState: &state}
	ctx := context.Background()

	args := strings.Split("connect --addr 1.2.3.4 --peer.tag=123hey"+
		" --misc.bad=false --misc.awesome --fork.digests=a1b2c3,42e5f6,a1b2c3"+
		" --foobar=2,0x123,-1,8 --hex 0x1234567890"+
		" somepeerid 42 optionalhere extra more", " ")
	if err := Run(ctx, cmd, args); err != nil {
		panic(err)
	}
	if state.HostData != "done!" {
		panic("expected state mutation")
	}
	// Output:
	// addr: 1.2.3.4
	// port: 9000
	// tag: 123hey
	// data: 42
	// fork opts more: optionalhere
	// remaining args: [extra more]
	// digests: a1b2c3!42e5f6!a1b2c3!
	// awesome: true
	// bad: false
	// foobar: [2 291 -1 8]
	// bytes: [18 52 86 120 144]
}

func ExampleUsageFromErr_peer_connect_help() {
	state := ActorState{
		HostData: "old value",
	}
	cmd := &Peer{ActorState: &state}
	ctx := context.Background()
	err := Run(ctx, cmd, []string{"connect", "--help"})
	if err == nil {
		panic("expected err, but got nil")
	}
	if !errors.Is(err, HelpErr) {
		panic(fmt.Errorf("expected help err, but got: %w", err))
	}
	usage := UsageFromErr(err)
	fmt.Println(usage)
	// Output:
	// peer connect <peer.id> <misc.data> [fork.more] # 8 flags (see below)
	//
	// Connect to a peer
	//
	//   --addr                      address to connect to (default: <nil>) (type: ip) (env: ADDR)
	//   --port                      port to use for connection (default: 9000) (type: uint16) (env: PORT)
	//   --foobar                    foobar integers (default: 4,5,6) (type: int32Slice) (env: FOOBAR)
	//   --hex                       Hex value (type: bytes) (env: HEX)
	//
	// # peer
	// Options for peer stuff
	//
	//   --peer.tag                  tag to give to peer (type: string) (env: PEER_TAG)
	//   <peer.id>                   libp2p ID of the peer, if no address is specified, the peer is looked up in the peerstore (type: string) (env: PEER_ID)
	//
	// # misc
	// Misc. options
	//
	//   <misc.data>                 some number (default: 0) (type: uint8) (env: MISC_DATA)
	//   --misc.awesome              Enable awesome feature (default: false) (type: bool) (env: MISC_AWESOME)
	//   --misc.bad                  Enable bad feature (default: true) (type: bool) (env: MISC_BAD)
	//
	// # fork
	// Fork options
	//
	//   --fork.digests              some digests (default: a1b2c3,d4e5f6) (type: []bytes3) (env: FORK_DIGESTS)
	//   [fork.more]                 something optional (type: string) (env: FORK_MORE)
	//
}

func ExampleUsageFromErr_peer_help() {
	state := ActorState{
		HostData: "old value",
	}
	cmd := &Peer{ActorState: &state}
	ctx := context.Background()
	err := Run(ctx, cmd, []string{"--help"})
	if err == nil {
		panic("expected err, but got nil")
	}
	if !errors.Is(err, HelpErr) {
		panic(fmt.Errorf("expected help err, but got: %w", err))
	}
	usage := UsageFromErr(err)
	fmt.Println(usage)
	// Output:
	// peer
	//
	// Sub commands:
	//   connect  Connect to a peer
}

type EnvTestCommand struct {
	Test    string `ask:"--env-test" help:"test env handling"`
	Double  string `ask:"--double" help:"for testing flag and env collision"`
	Ignored string `ask:"--this-is-flag-only" help:"flag only" env:"-"`
	Other   string `ask:"--other" help:"other flag, to check missing env var"`
	Nested  struct {
		Foo    string `ask:"--hello"`
		No     string `ask:"-"`
		Custom string `ask:"--custom" env:"SPECIAL_FLAG"`
	} `ask:".nest"`
}

func (c *EnvTestCommand) Run(ctx context.Context) error {
	fmt.Println("test:", c.Test)
	fmt.Println("double:", c.Double)
	fmt.Printf("ignored: %q\n", c.Ignored)
	fmt.Println("nested foo:", c.Nested.Foo)
	fmt.Printf("nested no: %q\n", c.Nested.No)
	fmt.Println("nested custom:", c.Nested.Custom)
	return nil
}

func ExampleWithEnvFn() {
	cmd := &EnvTestCommand{}
	env := map[string]string{
		"ENV_TEST":          "123",
		"DOUBLE":            "ENVED",
		"THIS_IS_FLAG_ONLY": "should be ignored",
		"NEST_HELLO":        "foobar",
		"NEST__":            "hmm",
		"NEST_":             "hmmm",
		"NEST":              "hmm",
		"SPECIAL_FLAG":      "escapes nesting with specific env var name",
	}
	envFn := EnvFn(func(key string) (value string, ok bool) {
		value, ok = env[key]
		return
	})
	ctx := WithEnvFn(context.Background(), envFn)
	err := Run(ctx, cmd, []string{"--double=FLAGGED"})
	if err != nil {
		panic(err)
	}
	// Output:
	// test: 123
	// double: FLAGGED
	// ignored: ""
	// nested foo: foobar
	// nested no: ""
	// nested custom: escapes nesting with specific env var name
	//
}
