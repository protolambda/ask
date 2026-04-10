package ask

import (
	"errors"
	"fmt"
)

var HelpErr = errors.New("ask: help asked with flag")

// UnrecognizedErr can be used to signal a sub-command was not recognized.
var UnrecognizedErr = errors.New("command was not recognized")

// cmdDescrError is used to wrap around errors that exit a Run scope,
// to annotate with command information.
// A stack of commands is tracked,
// to tell which sub-command an error originated from,
// and which commands it passed through.
type cmdDescrError struct {
	// Stack of commands that failed.
	// The most inner sub-command that failed is at 0,
	// And each newDescrError then appends the outer command that surrounds it.
	// This helps retrieve the description of the most inner sub-command,
	// to e.g. print usage info upon a HelpErr.
	Stack []*cmdDescription
	// Inner is the error that was wrapped
	Inner error
}

func newDescrError(inner error, descr *cmdDescription) *cmdDescrError {
	var prev *cmdDescrError
	if errors.As(inner, &prev) {
		newStack := make([]*cmdDescription, 0, len(prev.Stack)+1)
		newStack = append(newStack, prev.Stack...)
		newStack = append(newStack, descr)
		return &cmdDescrError{
			Stack: newStack,
			Inner: inner,
		}
	}
	return &cmdDescrError{
		Stack: []*cmdDescription{descr},
		Inner: inner,
	}
}

var _ error = (*cmdDescrError)(nil)

func (d *cmdDescrError) Error() string {
	descr := "?"
	if len(d.Stack) > 0 {
		descr = d.Stack[0].String()
	}
	return fmt.Sprintf("%s command error: %s", descr, d.Inner.Error())
}

func (d *cmdDescrError) Unwrap() error {
	return d.Inner
}
