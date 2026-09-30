//go:build !plan9

package ask

import (
	"os"
	"syscall"
)

// shutdownSignals are the signals RunProgram handles.
var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// signalExitCode is the exit status of a process that ends by sig where it cannot be killed by sig:
// 128 plus the signal number, the status a shell reports for a process killed by that signal.
func signalExitCode(sig os.Signal) int {
	if s, ok := sig.(syscall.Signal); ok {
		return 128 + int(s)
	}
	return 1
}
