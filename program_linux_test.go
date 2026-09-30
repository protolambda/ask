package ask

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"syscall"
	"testing"
	"time"
)

// pid1Attr starts a process as PID 1 (the init) of a new PID namespace, like in a container without an init.
// The new user namespace lets an unprivileged user create the PID namespace.
func pid1Attr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Cloneflags:  syscall.CLONE_NEWUSER | syscall.CLONE_NEWPID,
		UidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
	}
}

func TestRunProgram_processPID1(t *testing.T) {
	// User namespaces may be disabled, or blocked, e.g. by the seccomp profile of a container.
	probe := exec.Command(os.Args[0], "-test.run=^$")
	probe.SysProcAttr = pid1Attr()
	if err := probe.Run(); err != nil {
		t.Skipf("cannot start a process as PID 1 of a new PID namespace: %v", err)
	}
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		for _, second := range []bool{false, true} {
			sig, second := sig, second
			t.Run(fmt.Sprintf("%v/second=%v", sig, second), func(t *testing.T) {
				t.Parallel()
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				var args []string
				if second {
					args = append(args, "--stuck")
				}
				cmd, stdout, stderr := startProgramHelperAttr(t, ctx, helperCommand(args...), pid1Attr())
				if err := cmd.Process.Signal(sig); err != nil {
					t.Fatal(err)
				}
				want := []string{"cancelled", "closed"}
				if second {
					awaitLine(t, stdout, "cancelled")
					if err := cmd.Process.Signal(sig); err != nil {
						t.Fatal(err)
					}
					want = nil
				}
				rest := readLines(stdout)
				// The kernel does not apply the default action of a signal that PID 1 sends to itself,
				// so the process exits with the status instead, whatever the Go version does then.
				if ws, code := wait(t, cmd), 128+int(sig); ws.Signaled() || ws.ExitStatus() != code {
					t.Fatalf("expected exit status %d, got %s, stderr:\n%s", code, describe(ws), stderr)
				}
				if !slices.Equal(rest, want) {
					t.Errorf("expected output %q after the signal, got %q", want, rest)
				}
			})
		}
	}
}
