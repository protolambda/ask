//go:build unix

package ask

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

// raiseTimeout bounds how long raise waits for the raised signal to end the process.
const raiseTimeout = time.Second

// raise ends the process by sig, as the default action of sig does:
// it restores the default handling of sig, and sends sig to the process.
// It returns if the process survives that, e.g. if sig was ignored when the program started,
// and at once as PID 1; the caller then exits with a status instead.
func raise(sig os.Signal) {
	s, ok := sig.(syscall.Signal)
	if !ok {
		return
	}
	// The kernel does not apply a default action to a signal that PID 1 (the init of a PID namespace,
	// e.g. a container without an init) sends to itself. The Go runtime would then exit by itself,
	// with status 2 up to Go 1.25.
	if syscall.Getpid() == 1 {
		return
	}
	signal.Reset(s)
	if err := syscall.Kill(syscall.Getpid(), s); err != nil {
		return
	}
	// The signal may be delivered to another thread: give it time to end the process.
	time.Sleep(raiseTimeout)
}
