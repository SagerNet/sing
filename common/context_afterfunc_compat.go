//go:build go1.20 && !go1.21

package common

import (
	"context"
	"sync"
)

// ContextAfterFunc arranges to call f in its own goroutine after ctx is done.
// Returns a stop function that prevents f from being run.
func ContextAfterFunc(ctx context.Context, f func()) (stop func() bool) {
	stopCh := make(chan struct{})
	var once sync.Once
	stopped := false

	go func() {
		select {
		case <-ctx.Done():
			once.Do(func() {
				if !stopped {
					f()
				}
			})
		case <-stopCh:
		}
	}()

	return func() bool {
		select {
		case <-ctx.Done():
			return false
		default:
			stopped = true
			once.Do(func() {
				close(stopCh)
			})
			return true
		}
	}
}
