//go:build !unix

package ask

import "os"

// raise does nothing where a signal cannot be raised again with its default action:
// the caller exits the process with a status instead.
func raise(os.Signal) {}
