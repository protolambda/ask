package ask

import "context"

// IsSet checks if the given flag is set within the current Run scope.
// Flags of outer Run scopes are ignored.
func IsSet(ctx context.Context, flag string) bool {
	li := getCmdLink(ctx)
	if li == nil {
		return false
	}
	_, ok := li.Descr.SeenFlags[flag]
	return ok
}
