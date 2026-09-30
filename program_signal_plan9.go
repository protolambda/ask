package ask

import "os"

// shutdownSignals are the signals RunProgram handles.
// Plan 9 has no SIGTERM, so only os.Interrupt is handled there.
var shutdownSignals = []os.Signal{os.Interrupt}

// signalExitCode is the exit status of a process that ends by sig.
// Plan 9 signals are notes, without a number.
func signalExitCode(os.Signal) int {
	return 1
}
