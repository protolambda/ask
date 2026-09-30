package ask

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// programCmd is a command for runProgram tests.
type programCmd struct {
	Fail bool `ask:"--fail" help:"Fail the command"`
	// CtxErr makes the command return the error of its context, e.g. context.Canceled.
	CtxErr bool `ask:"--ctx-err" help:"Return the context error"`
	// Wait makes the command wait for cancellation before it returns.
	Wait bool `ask:"--wait" help:"Wait for cancellation"`
	// Stuck makes the command wait for cancellation, and then for release, like a command stuck in cleanup.
	Stuck bool `ask:"--stuck" help:"Keep running after cancellation, until released"`
	// CloseFail makes Close fail, like a cleanup that could not flush.
	CloseFail bool `ask:"--close-fail" help:"Fail to close"`
	// ClosePanic makes Close panic.
	ClosePanic bool `ask:"--close-panic" help:"Panic in Close"`

	cancelled chan struct{}
	release   chan struct{}
	closed    chan struct{}
}

func newProgramCmd() *programCmd {
	return &programCmd{
		cancelled: make(chan struct{}),
		release:   make(chan struct{}),
		closed:    make(chan struct{}),
	}
}

func (c *programCmd) Run(ctx context.Context) error {
	if c.Wait || c.Stuck {
		<-ctx.Done()
		close(c.cancelled)
	}
	if c.Stuck {
		<-c.release
	}
	switch {
	case c.Fail && c.CtxErr:
		// Like a command that joins the errors of its workers: one failed, the others were cancelled.
		return errors.Join(errors.New("command failed"), ctx.Err())
	case c.Fail:
		return errors.New("command failed")
	case c.CtxErr:
		return ctx.Err()
	}
	return nil
}

func (c *programCmd) Close() error {
	close(c.closed)
	if c.ClosePanic {
		panic("close panicked")
	}
	if c.CloseFail {
		return errors.New("flush failed")
	}
	return nil
}

// isClosed reports whether ch is closed, without waiting.
func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestRunProgram_exitStatus(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{name: "success", code: 0},
		{name: "failure", args: []string{"--fail"}, code: 1, stderr: "failed to run: command failed"},
		{name: "unknown flag", args: []string{"--foo"}, code: 1, stderr: "failed to apply command arguments"},
		{name: "help", args: []string{"--help"}, code: 0, stderr: "--fail"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			var stderr strings.Builder
			code, sig := runProgram(newProgramCmd(), c.args, &stderr, nil)
			if code != c.code || sig != nil {
				t.Errorf("expected exit status %d, got %d and signal %v, stderr:\n%s", c.code, code, sig, &stderr)
			}
			if c.stderr == "" && stderr.Len() != 0 {
				t.Errorf("expected no stderr output, got %q", &stderr)
			}
			if !strings.Contains(stderr.String(), c.stderr) {
				t.Errorf("stderr %q does not contain %q", &stderr, c.stderr)
			}
		})
	}
}

func TestRunProgram_signalCancels(t *testing.T) {
	for _, sig := range shutdownSignals {
		sig := sig
		t.Run(sig.String(), func(t *testing.T) {
			signals := make(chan os.Signal, 1)
			signals <- sig
			cmd := newProgramCmd()
			var stderr strings.Builder
			// The command only returns once the signal cancelled it, and then returns nil:
			// the process ends by the signal, not with a success.
			code, got := runProgram(cmd, []string{"--wait"}, &stderr, signals)
			if got != sig {
				t.Errorf("expected to end by signal %v, got %v", sig, got)
			}
			if want := signalExitCode(sig); code != want || code == 0 {
				t.Errorf("expected non-zero exit status %d, got %d", want, code)
			}
			if !isClosed(cmd.closed) {
				t.Error("command was not closed")
			}
			if want := sig.String() + ": shutting down, signal again to exit immediately\n"; stderr.String() != want {
				t.Errorf("expected stderr %q, got %q", want, &stderr)
			}
		})
	}
}

func TestRunProgram_signalCanceledError(t *testing.T) {
	for _, sig := range shutdownSignals {
		sig := sig
		t.Run(sig.String(), func(t *testing.T) {
			signals := make(chan os.Signal, 1)
			signals <- sig
			var stderr strings.Builder
			// A command that returns context.Canceled after the signal ends by the signal too, not with a failure.
			code, got := runProgram(newProgramCmd(), []string{"--wait", "--ctx-err"}, &stderr, signals)
			if got != sig {
				t.Errorf("expected to end by signal %v, got %v", sig, got)
			}
			if want := signalExitCode(sig); code != want {
				t.Errorf("expected exit status %d, got %d", want, code)
			}
			// The error is still printed.
			if want := "context canceled"; !strings.Contains(stderr.String(), want) {
				t.Errorf("stderr %q does not contain %q", &stderr, want)
			}
		})
	}
}

func TestRunProgram_signalFailure(t *testing.T) {
	signals := make(chan os.Signal, 1)
	signals <- os.Interrupt
	var stderr strings.Builder
	// A command that fails on cancellation still exits with status 1.
	code, sig := runProgram(newProgramCmd(), []string{"--wait", "--fail"}, &stderr, signals)
	if code != 1 || sig != nil {
		t.Errorf("expected exit status 1, got %d and signal %v, stderr:\n%s", code, sig, &stderr)
	}
	if want := "command failed"; !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr %q does not contain %q", &stderr, want)
	}
}

func TestRunProgram_signalCleanupFailure(t *testing.T) {
	// A failure after a shutdown signal gives exit status 1, also when context.Canceled is joined to it:
	// a cleanup failure must reach the parent, whether the command returned nil or ctx.Err().
	cases := []struct {
		name   string
		args   []string
		stderr string
	}{
		{name: "close fails after nil", args: []string{"--close-fail"}, stderr: "failed to close: flush failed"},
		{name: "close fails after ctx.Err()", args: []string{"--ctx-err", "--close-fail"}, stderr: "failed to close: flush failed"},
		{name: "close panics after ctx.Err()", args: []string{"--ctx-err", "--close-panic"}, stderr: "panicked with message: close panicked"},
		{name: "failure joined with ctx.Err()", args: []string{"--ctx-err", "--fail"}, stderr: "command failed"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			signals := make(chan os.Signal, 1)
			signals <- os.Interrupt
			var stderr strings.Builder
			code, sig := runProgram(newProgramCmd(), append([]string{"--wait"}, c.args...), &stderr, signals)
			if code != 1 || sig != nil {
				t.Errorf("expected exit status 1, got %d and signal %v, stderr:\n%s", code, sig, &stderr)
			}
			if !strings.Contains(stderr.String(), c.stderr) {
				t.Errorf("stderr %q does not contain %q", &stderr, c.stderr)
			}
		})
	}
}

// canceledErr is an error that declares itself a cancellation with an Is method.
type canceledErr struct{}

func (canceledErr) Error() string        { return "stopped" }
func (canceledErr) Is(target error) bool { return target == context.Canceled }

func TestCanceledOnly(t *testing.T) {
	failed := errors.New("flush failed")
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "canceled", err: context.Canceled, want: true},
		{name: "wrapped", err: fmt.Errorf("failed to run: %w", context.Canceled), want: true},
		{name: "joined once", err: errors.Join(fmt.Errorf("failed to run: %w", context.Canceled)), want: true},
		{name: "joined cancellations", err: errors.Join(context.Canceled, fmt.Errorf("worker: %w", context.Canceled)), want: true},
		{name: "Is method", err: fmt.Errorf("worker: %w", canceledErr{}), want: true},
		{name: "joined failure", err: errors.Join(fmt.Errorf("failed to run: %w", context.Canceled), failed), want: false},
		{name: "nested joined failure", err: fmt.Errorf("cmd: %w", errors.Join(errors.Join(context.Canceled), failed)), want: false},
		{name: "several %w", err: fmt.Errorf("%w: %w", failed, context.Canceled), want: false},
		{name: "other error", err: failed, want: false},
		{name: "deadline", err: context.DeadlineExceeded, want: false},
		{name: "same text", err: errors.New(context.Canceled.Error()), want: false},
		{name: "flattened", err: fmt.Errorf("failed to run: %v", context.Canceled), want: false},
	}
	for _, c := range cases {
		if got := canceledOnly(c.err); got != c.want {
			t.Errorf("%s: canceledOnly(%v) = %v, expected %v", c.name, c.err, got, c.want)
		}
	}
}

func TestRunProgram_secondSignalExits(t *testing.T) {
	for _, sig := range shutdownSignals {
		sig := sig
		t.Run(sig.String(), func(t *testing.T) {
			signals := make(chan os.Signal, 2)
			signals <- sig
			signals <- sig
			cmd := newProgramCmd()
			defer close(cmd.release) // let the command return after the test
			var stderr strings.Builder
			type result struct {
				code int
				sig  os.Signal
			}
			results := make(chan result, 1)
			go func() {
				code, sig := runProgram(cmd, []string{"--stuck"}, &stderr, signals)
				results <- result{code, sig}
			}()
			var res result
			select {
			case res = <-results:
			case <-time.After(10 * time.Second):
				t.Fatal("runProgram did not return upon the second signal")
			}
			if res.sig != sig {
				t.Errorf("expected to end by signal %v, got %v", sig, res.sig)
			}
			if want := signalExitCode(sig); res.code != want || res.code == 0 {
				t.Errorf("expected non-zero exit status %d, got %d", want, res.code)
			}
			if isClosed(cmd.closed) {
				t.Error("command completed, but should still be running")
			}
			want := sig.String() + ": shutting down, signal again to exit immediately\n" +
				sig.String() + ": exiting immediately\n"
			if stderr.String() != want {
				t.Errorf("expected stderr %q, got %q", want, &stderr)
			}
		})
	}
}
