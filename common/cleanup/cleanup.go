package cleanup

import (
	"runtime"
	"sync"
	_ "unsafe"
)

func init() {
	registerPoolCleanup(myCleanup)
}

//go:linkname registerPoolCleanup sync.runtime_registerPoolCleanup
func registerPoolCleanup(cleanup func())

//go:linkname poolCleanup sync.poolCleanup
func poolCleanup()

var unsafeCleanupFuncs []func()

func myCleanup() {
	poolCleanup()
	for _, cleanupFunc := range unsafeCleanupFuncs {
		cleanupFunc()
	}
}

// AddUnsafe must be called only in init {}
// called in STW, must not allocate or hold a lock
func AddUnsafe(cleanup func()) {
	unsafeCleanupFuncs = append(unsafeCleanupFuncs, cleanup)
}

type Cleaner struct {
	access sync.Mutex
	state  *cleanupState
}

func Add(cleanup func()) *Cleaner {
	state := &cleanupState{
		cleanupFunc: cleanup,
	}
	newObject(state)
	return &Cleaner{
		state: state,
	}
}

func (c *Cleaner) Close() {
	c.access.Lock()
	defer c.access.Unlock()
	if c.state == nil {
		return
	}
	c.state.close()
	c.state = nil
}

type cleanupState struct {
	access      sync.Mutex
	cleanupFunc func()
	closed      bool
}

func (s *cleanupState) close() {
	s.access.Lock()
	defer s.access.Unlock()
	s.closed = true
}

type Object struct {
	state *cleanupState
}

func newObject(state *cleanupState) {
	object := &Object{
		state: state,
	}
	runtime.SetFinalizer(object, (*Object).cleanup)
}

func (o *Object) cleanup() {
	state := o.state
	state.access.Lock()
	if state.closed {
		state.access.Unlock()
		return
	}

	newObject(state)
	cleanupFunc := state.cleanupFunc
	state.access.Unlock()
	cleanupFunc()
}
