package ask

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Run parses, runs and closes completion (cmd.Close unconditionally after successful/failed cmd.Run).
func Run(ctx context.Context, cmd Command, args []string, opts ...Option) (outErr error) {
	var descr *cmdDescription
	// Wrap with Descr, so we can print the usage,
	// and have a trace of which command failed.
	defer func() {
		if descr != nil && outErr != nil {
			outErr = newDescrError(outErr, descr)
		}
	}()
	// wrap with panic-guard
	defer func() {
		if v := recover(); v != nil {
			if e, ok := v.(error); ok {
				outErr = errors.Join(outErr, fmt.Errorf("panicked with error: %w", e))
			} else {
				outErr = errors.Join(outErr, fmt.Errorf("panicked with message: %v", v))
			}
		}
	}()

	cfg := newRunConfig(opts...)

	// Parse the command into a fully described command
	if v, err := loadCmdDescription(cmd, cfg); err != nil {
		outErr = errors.Join(outErr, fmt.Errorf("failed to load command %T: %w", cmd, err))
		return outErr
	} else {
		descr = v
	}

	// Apply the defaults to the command
	if v, ok := cmd.(InitDefault); ok {
		v.Default()
	}

	// Apply the args: fill flags and remaining args with values
	// This returns a HelpErr when the user specified -h/--help,
	// or UnrecognizedErr when flags don't match.
	if err := descr.applyArgs(ctx, args); err != nil {
		return fmt.Errorf("failed to apply command arguments: %w", err)
	}

	ctx = addCmdLink(ctx, descr, cmd)
	defer func() {
		if c, ok := cmd.(io.Closer); ok {
			if err := c.Close(); err != nil {
				outErr = errors.Join(outErr, fmt.Errorf("failed to close: %w", err))
			}
		}
	}()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := cmd.Run(ctx); err != nil {
		outErr = errors.Join(outErr, fmt.Errorf("failed to run: %w", err))
	}

	return outErr
}
