//nolint:paralleltest
package cleanup

import (
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCleanup(t *testing.T) {
	var didCleanup atomic.Int32
	Add(func() {
		didCleanup.Add(1)
	})
	runCleanup(t, func() bool {
		return didCleanup.Load() >= 1
	})
	runCleanup(t, func() bool {
		return didCleanup.Load() >= 2
	})
}

func TestCleanupUnsafe(t *testing.T) {
	var didCleanup atomic.Int32
	AddUnsafe(func() {
		didCleanup.Add(1)
	})
	runCleanup(t, func() bool {
		return didCleanup.Load() >= 1
	})
	runCleanup(t, func() bool {
		return didCleanup.Load() >= 2
	})
}

func TestCleanup1(t *testing.T) {
	var didCleanup atomic.Bool
	var didReset atomic.Bool
	m := newMap(func() {
		didCleanup.Store(true)
	})
	Add(func() {
		*m = sync.Map{}
		didReset.Store(true)
	})
	runtime.KeepAlive(&m)
	runCleanup(t, func() bool {
		return didReset.Load()
	})
	require.False(t, didCleanup.Load())
	runCleanup(t, func() bool {
		return didCleanup.Load()
	})
}

func TestCleanup1Unsafe(t *testing.T) {
	var didCleanup atomic.Bool
	m := newMap(func() {
		didCleanup.Store(true)
	})
	AddUnsafe(func() {
		*m = sync.Map{}
	})
	runtime.KeepAlive(&m)
	runCleanup(t, func() bool {
		return didCleanup.Load()
	})
}

type myObj struct {
	_ string
}

func newMap(cleanup func()) *sync.Map {
	obj := &myObj{}
	runtime.SetFinalizer(obj, func(*myObj) {
		cleanup()
	})
	var m sync.Map
	m.Store("test", obj)
	return &m
}

func TestSafeCleanup(t *testing.T) {
	var didCleanup atomic.Int32
	safeObject := Add(func() {
		didCleanup.Add(1)
	})
	runCleanup(t, func() bool {
		return didCleanup.Load() >= 1
	})
	safeObject.Close()
	didCleanupAfterClose := didCleanup.Load()
	debug.FreeOSMemory()
	require.Never(t, func() bool {
		return didCleanup.Load() != didCleanupAfterClose
	}, cleanupWait, cleanupTick)
}

func TestCloseSkipsQueuedCleanup(t *testing.T) {
	blockCleanup := make(chan struct{})
	cleanupBlocked := make(chan struct{})
	var blockOnce sync.Once
	blocker := Add(func() {
		blockOnce.Do(func() {
			close(cleanupBlocked)
			<-blockCleanup
		})
	})
	runCleanup(t, func() bool {
		select {
		case <-cleanupBlocked:
			return true
		default:
			return false
		}
	})

	var didCleanup atomic.Bool
	queued := Add(func() {
		didCleanup.Store(true)
	})
	debug.FreeOSMemory()
	queued.Close()

	close(blockCleanup)
	blocker.Close()
	require.Never(t, didCleanup.Load, cleanupWait, cleanupTick)
}

func TestCleanupCanCloseItself(t *testing.T) {
	ready := make(chan struct{})
	done := make(chan struct{})
	var cleaner *Cleaner
	cleaner = Add(func() {
		<-ready
		cleaner.Close()
		close(done)
	})
	close(ready)

	runCleanup(t, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	})
}

const (
	cleanupWait = time.Second
	cleanupTick = time.Millisecond
)

func runCleanup(t *testing.T, condition func() bool) {
	t.Helper()
	debug.FreeOSMemory()
	require.Eventually(t, condition, cleanupWait, cleanupTick)
}
