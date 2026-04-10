package ask

import (
	"context"
	"os"
	"strings"
)

type lookupEnvCtxKeyType struct{}

var lookupEnvCtxKey = lookupEnvCtxKeyType{}

// EnvFn looks up an environment variable like os.LookupEnv.
// Environment variable lookups can be customized per context with WithEnvFn.
type EnvFn func(key string) (value string, ok bool)

// WithEnvFn sets a contextual EnvFn to use.
func WithEnvFn(ctx context.Context, fn EnvFn) context.Context {
	return context.WithValue(ctx, lookupEnvCtxKey, fn)
}

// EnvFnFromContext determines the EnvFn to use.
// If none was specified in the context, os.LookupEnv is used.
func EnvFnFromContext(ctx context.Context) EnvFn {
	v := ctx.Value(lookupEnvCtxKey)
	if v == nil {
		return os.LookupEnv
	}
	return v.(EnvFn)
}

// FlagPathToEnvKey infers an env var from a flag path
func FlagPathToEnvKey(path string) (key string) {
	key = path
	key = strings.ReplaceAll(key, ".", "_")
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ToUpper(key)
	return key
}
