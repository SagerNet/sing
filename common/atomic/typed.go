package atomic

import (
	"sync/atomic"

	"github.com/sagernet/sing/common"
)

type TypedValue[T any] struct {
	value atomic.Value
}

// tValue is a struct with determined type to resolve atomic.Value usages with interface types
// https://github.com/golang/go/issues/22550
//
// The intention to have an atomic value store for errors. However, running this code panics:
// panic: sync/atomic: store of inconsistently typed value into Value
// This is because atomic.Value requires that the underlying concrete type be the same (which is a reasonable expectation for its implementation).
// When going through the atomic.Value.Store method call, the fact that both these are of the error interface is lost.
type tValue[T any] struct {
	value T
}

func (t *TypedValue[T]) Load() T {
	value := t.value.Load()
	if value == nil {
		return common.DefaultValue[T]()
	}
	return value.(tValue[T]).value
}

func (t *TypedValue[T]) Store(value T) {
	t.value.Store(tValue[T]{value})
}

func (t *TypedValue[T]) Swap(new T) T {
	old := t.value.Swap(tValue[T]{new})
	if old == nil {
		return common.DefaultValue[T]()
	}
	return old.(tValue[T]).value
}

func (t *TypedValue[T]) CompareAndSwap(old, new T) bool {
	return t.value.CompareAndSwap(tValue[T]{old}, tValue[T]{new})
}
