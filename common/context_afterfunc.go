//go:build go1.21

package common

import "context"

// ContextAfterFunc arranges to call f in its own goroutine after ctx is done.
// Returns a stop function that prevents f from being run.
func ContextAfterFunc(ctx context.Context, f func()) (stop func() bool) {
	return context.AfterFunc(ctx, f)
}
