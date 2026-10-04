package config

import (
	"context"
	"os"
)

type envKey struct{}

// WithEnv returns a context whose config reads take every environment variable
// from lookup instead of the process, so a run can be handed its own
// credentials and config directory.
func WithEnv(ctx context.Context, lookup func(string) (string, bool)) context.Context {
	return context.WithValue(ctx, envKey{}, lookup)
}

// getenv reads name the way os.Getenv does: unset and empty are the same.
func getenv(ctx context.Context, name string) string {
	lookup, ok := ctx.Value(envKey{}).(func(string) (string, bool))
	if !ok {
		lookup = os.LookupEnv
	}

	value, _ := lookup(name)

	return value
}
