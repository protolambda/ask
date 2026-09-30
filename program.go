package ask

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
)

// RunProgram is a convenience-method to run a command as the main function of a program,
// and exit the process when the command completes.
// Arguments are read from os.Args[1:] (i.e. program name is skipped).
// Set the "HIDDEN_OPTIONS" env var to show hidden CLI options.
//
// A shutdown signal (os.Interrupt, or SIGTERM on platforms that have it) cancels the context of the command,
// so it can clean up and return; Close still runs if the command implements io.Closer.
// If the command then stops without a failure, i.e. it returns nil or an error that wraps nothing but
// context.Canceled (such as ctx.Err()), and Close does not fail, the process ends by that signal,
// as it would without the signal handling, so its parent sees that it was stopped early:
// a shell stops a script upon Ctrl-C, and a batch runner does not take the work for done.
// A second shutdown signal ends the process by that signal immediately, without waiting for the command,
// e.g. when its cleanup hangs.
// On Unix the signal is raised again with its default action (a shell reports the status 128 plus the signal
// number: 130 for os.Interrupt, 143 for SIGTERM). Elsewhere, as PID 1 (e.g. in a container without an init),
// or if the signal was ignored when the program started (as a shell does with SIGINT for a background job),
// the process exits with that status instead (1 on Plan 9).
//
// Otherwise the exit status is 0 when the command succeeds or help is requested (the usage is printed to stderr),
// and 1 when the command or Close fails (the error is printed to stderr), also after a shutdown signal,
// even if context.Canceled is joined to the failure.
func RunProgram(cmd Command) {
	// Room for a second signal that arrives while the first one is handled.
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, shutdownSignals...)

	opts := []Option{
		OnDeprecated(func(ctx context.Context, fl PrefixedFlag) error {
			_, _ = fmt.Fprintf(os.Stderr, "warning: flag %q is deprecated: %s\n", fl.Path, fl.Deprecated)
			return nil
		}),
		ShowHidden(os.Getenv("HIDDEN_OPTIONS") != ""),
	}
	code, sig := runProgram(cmd, os.Args[1:], os.Stderr, signals, opts...)
	if sig != nil {
		raise(sig)
	}
	os.Exit(code)
}

// runProgram implements RunProgram, with the process I/O injected: it runs cmd with args,
// cancels it upon a signal, and returns the exit status.
// If sig is not nil, the process should end by that signal, and code is the exit status to use where it cannot.
// Upon a second signal it returns that signal at once, while the command may still run.
func runProgram(cmd Command, args []string, stderr io.Writer, signals <-chan os.Signal, opts ...Option) (code int, sig os.Signal) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// run command in the background, so we can stop it at any time.
	// Buffered, so the command can still complete after a forced exit.
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, cmd, args, opts...)
	}()

	var shutdown os.Signal // the signal that cancelled the command, if any
	for {
		select {
		case err := <-done:
			switch {
			case errors.Is(err, HelpErr):
				_, _ = fmt.Fprintln(stderr, UsageFromErr(err))
				return 0, nil
			case shutdown != nil && (err == nil || canceledOnly(err)):
				// The command stopped because of the signal: report the signal, not a success or a failure.
				if err != nil {
					_, _ = fmt.Fprintln(stderr, err.Error())
				}
				return signalExitCode(shutdown), shutdown
			case err == nil:
				return 0, nil
			default:
				_, _ = fmt.Fprintln(stderr, err.Error())
				return 1, nil
			}
		case sig := <-signals:
			if shutdown != nil {
				_, _ = fmt.Fprintf(stderr, "%v: exiting immediately\n", sig)
				return signalExitCode(sig), sig
			}
			shutdown = sig
			cancel()
			_, _ = fmt.Fprintf(stderr, "%v: shutting down, signal again to exit immediately\n", sig)
		}
	}
}

// canceledOnly reports whether err is context.Canceled, or wraps only errors that are:
// unlike errors.Is, it requires every error joined in err (errors.Join, or fmt.Errorf with several %w)
// to match, so a failure joined with a cancellation, e.g. by a Close that fails after the command
// returned ctx.Err(), is not taken for a cancellation.
func canceledOnly(err error) bool {
	if err == nil {
		return false
	}
	if err == context.Canceled {
		return true
	}
	if x, ok := err.(interface{ Is(error) bool }); ok && x.Is(context.Canceled) {
		return true
	}
	switch x := err.(type) {
	case interface{ Unwrap() error }:
		return canceledOnly(x.Unwrap())
	case interface{ Unwrap() []error }:
		errs := x.Unwrap()
		for _, e := range errs {
			if !canceledOnly(e) {
				return false
			}
		}
		return len(errs) > 0
	}
	return false
}
