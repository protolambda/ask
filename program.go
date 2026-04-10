package ask

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
)

// RunProgram is a convenience-method to run a command:
// it handles long-running commands and shuts down on os.Interrupt signal.
// Arguments are read from os.Args[1:] (i.e. program name is skipped).
// Set the "HIDDEN_OPTIONS" env var to show hidden CLI options.
func RunProgram(cmd Command) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	ctx, cancel := context.WithCancel(context.Background())

	opts := []Option{
		OnDeprecated(func(ctx context.Context, fl PrefixedFlag) error {
			fmt.Fprintf(os.Stderr, "warning: flag %q is deprecated: %s", fl.Path, fl.Deprecated)
			return nil
		}),
		ShowHidden(os.Getenv("HIDDEN_OPTIONS") != ""),
	}

	starter := make(chan error)

	// run command in the background, so we can stop it at any time
	go func() {
		err := Run(ctx, cmd, os.Args[1:], opts...)
		starter <- err
	}()

	for {
		select {
		case err := <-starter:
			if err == nil {
				os.Exit(0)
			} else if errors.Is(err, HelpErr) {
				usage := UsageFromErr(err)
				_, _ = fmt.Fprintln(os.Stderr, usage)
				os.Exit(0)
			} else {
				_, _ = fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
			}
		case <-interrupt: // if interrupted, then we try to cancel
			cancel()
			_, _ = fmt.Fprintln(os.Stderr, "interrupted")
		}
	}
}
