package ask

import "context"

// Args get all arguments (those remaining after flags are handled)
func Args(ctx context.Context) []string {
	li := getCmdLink(ctx)
	if li == nil {
		return nil
	}
	return li.Descr.RemainingArgs
}

// SplitArgs returns args, split as first arg and all remaining args,
// as convenience for sub-command handling.
func SplitArgs(ctx context.Context) (first string, remaining []string) {
	args := Args(ctx)
	if len(args) == 0 {
		return "", nil
	}
	return args[0], args[1:]
}
