//go:build unix

package ask

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// programHelperEnv makes TestRunProgramHelper run RunProgram in a child process.
const programHelperEnv = "ASK_TEST_RUN_PROGRAM_HELPER"

// programHelperCmd is the command TestRunProgramHelper runs with RunProgram.
// It reports its progress on stdout, so the parent process knows when to signal it.
type programHelperCmd struct {
	CtxErr bool `ask:"--ctx-err" help:"Return the context error after cancellation"`
	Stuck  bool `ask:"--stuck" help:"Keep running after cancellation"`
}

func (c *programHelperCmd) Run(ctx context.Context) error {
	fmt.Println("ready")
	<-ctx.Done()
	fmt.Println("cancelled")
	if c.Stuck {
		select {} // ignore the cancellation, like a command stuck in cleanup
	}
	if c.CtxErr {
		return ctx.Err()
	}
	return nil
}

func (c *programHelperCmd) Close() error {
	fmt.Println("closed")
	return nil
}

// TestRunProgramHelper is not a test by itself: it is the child process of the TestRunProgram_process tests,
// and runs programHelperCmd with the arguments after "--".
func TestRunProgramHelper(t *testing.T) {
	if os.Getenv(programHelperEnv) == "" {
		return
	}
	os.Args = append([]string{"helper"}, flag.Args()...)
	RunProgram(&programHelperCmd{})
}

// helperCommand is the command line of a RunProgram process of programHelperCmd with args.
func helperCommand(args ...string) []string {
	return append([]string{os.Args[0], "-test.run=^TestRunProgramHelper$", "--"}, args...)
}

// startProgramHelper starts the command line argv, which runs a RunProgram process of programHelperCmd,
// and waits until the command runs (and thus handles signals).
func startProgramHelper(t *testing.T, ctx context.Context, argv []string) (cmd *exec.Cmd, stdout *bufio.Scanner, stderr *strings.Builder) {
	t.Helper()
	return startProgramHelperAttr(t, ctx, argv, nil)
}

// startProgramHelperAttr is startProgramHelper, with the OS-specific attributes of the process.
func startProgramHelperAttr(t *testing.T, ctx context.Context, argv []string, attr *syscall.SysProcAttr) (cmd *exec.Cmd, stdout *bufio.Scanner, stderr *strings.Builder) {
	t.Helper()
	cmd = exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), programHelperEnv+"=1")
	cmd.SysProcAttr = attr
	stderr = new(strings.Builder)
	cmd.Stderr = stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	stdout = bufio.NewScanner(out)
	awaitLine(t, stdout, "ready")
	return cmd, stdout, stderr
}

// awaitLine reads stdout until the given line.
func awaitLine(t *testing.T, stdout *bufio.Scanner, line string) {
	t.Helper()
	for stdout.Scan() {
		if stdout.Text() == line {
			return
		}
	}
	t.Fatalf("no %q line in the output: %v", line, stdout.Err())
}

func TestRunProgram_processSignals(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		for _, ctxErr := range []bool{false, true} {
			sig, ctxErr := sig, ctxErr
			t.Run(fmt.Sprintf("%v/ctxErr=%v", sig, ctxErr), func(t *testing.T) {
				t.Parallel()
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				var args []string
				if ctxErr {
					args = append(args, "--ctx-err")
				}
				cmd, stdout, stderr := startProgramHelper(t, ctx, helperCommand(args...))
				if err := cmd.Process.Signal(sig); err != nil {
					t.Fatal(err)
				}
				rest := readLines(stdout)
				if ws := wait(t, cmd); !endedBy(ws, sig) {
					t.Fatalf("expected the process to end by %v after a graceful shutdown, got %s, stderr:\n%s", sig, describe(ws), stderr)
				}
				if want := []string{"cancelled", "closed"}; !slices.Equal(rest, want) {
					t.Errorf("expected output %q after the signal, got %q", want, rest)
				}
				if want := sig.String() + ": shutting down"; !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr %q does not contain %q", stderr, want)
				}
			})
		}
	}
}

func TestRunProgram_processIgnoredSignal(t *testing.T) {
	t.Parallel()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to start the process with SIGINT ignored")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// A shell starts a background job with SIGINT ignored (and the ignored disposition survives exec):
	// the signal cannot end the process when raised again, so it exits with the status instead.
	argv := append([]string{sh, "-c", `trap "" INT; exec "$@"`, "sh"}, helperCommand()...)
	cmd, stdout, stderr := startProgramHelper(t, ctx, argv)
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	rest := readLines(stdout)
	if ws, want := wait(t, cmd), 128+int(syscall.SIGINT); ws.Signaled() || ws.ExitStatus() != want {
		t.Fatalf("expected exit status %d, got %s, stderr:\n%s", want, describe(ws), stderr)
	}
	if want := []string{"cancelled", "closed"}; !slices.Equal(rest, want) {
		t.Errorf("expected output %q after the signal, got %q", want, rest)
	}
}

func TestRunProgram_processSecondSignal(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		sig := sig
		t.Run(sig.String(), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			cmd, stdout, stderr := startProgramHelper(t, ctx, helperCommand("--stuck"))
			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			// Signal again only after the first signal was handled: the OS may merge pending signals.
			awaitLine(t, stdout, "cancelled")
			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			rest := readLines(stdout)
			// The process ends by the signal, as without the signal handling, so a shell stops a script too.
			if ws := wait(t, cmd); !endedBy(ws, sig) {
				t.Fatalf("expected the process to end by %v, got %s, stderr:\n%s", sig, describe(ws), stderr)
			}
			if len(rest) != 0 {
				t.Errorf("expected the command to still run at exit, got output %q", rest)
			}
			if want := sig.String() + ": exiting immediately"; !strings.Contains(stderr.String(), want) {
				t.Errorf("stderr %q does not contain %q", stderr, want)
			}
		})
	}
}

// readLines reads the remaining lines of stdout.
func readLines(stdout *bufio.Scanner) (lines []string) {
	for stdout.Scan() {
		lines = append(lines, stdout.Text())
	}
	return lines
}

// wait waits for the process to end, and returns its wait status.
func wait(t *testing.T, cmd *exec.Cmd) syscall.WaitStatus {
	t.Helper()
	var exitErr *exec.ExitError
	if err := cmd.Wait(); err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("unexpected wait error: %v", err)
	}
	return cmd.ProcessState.Sys().(syscall.WaitStatus)
}

// describe describes how a process ended.
func describe(ws syscall.WaitStatus) string {
	if ws.Signaled() {
		return "signal " + ws.Signal().String()
	}
	return fmt.Sprintf("exit status %d", ws.ExitStatus())
}

// endedBy reports whether a RunProgram process started by this test ended by sig, which it raises again.
// If sig is ignored in this process, e.g. SIGINT when the test binary runs as a background job of a
// non-interactive shell, the child inherits that and cannot end by sig: it exits with 128 plus the signal
// number instead, as documented (see TestRunProgram_processIgnoredSignal).
func endedBy(ws syscall.WaitStatus, sig syscall.Signal) bool {
	if signal.Ignored(sig) {
		return !ws.Signaled() && ws.ExitStatus() == 128+int(sig)
	}
	return ws.Signaled() && ws.Signal() == sig
}
