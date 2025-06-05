package atomic

import (
	"sync/atomic"

	"github.com/sagernet/sing/common"
)

type TypedValue[T any] struct {
	value atomic.Value
}

// typedValue is a struct with determined type to resolve atomic.Value usages with interface types
// https://github.com/golang/go/issues/22550
//
// The intention to have an atomic value store for errors. However, running this code panics:
// panic: sync/atomic: store of inconsistently typed value into Value
// This is because atomic.Value requires that the underlying concrete type be the same (which is a reasonable expectation for its implementation).
// When going through the atomic.Value.Store method call, the fact that both these are of the error interface is lost.
type typedValue[T any] struct {
	value T
}

func (t *TypedValue[T]) Load() T {
	value := t.value.Load()
	if value == nil {
		return common.DefaultValue[T]()
	}
	return value.(typedValue[T]).value
}

func (t *TypedValue[T]) Store(value T) {
	t.value.Store(typedValue[T]{value})
}

func (t *TypedValue[T]) Swap(new T) T {
	old := t.value.Swap(typedValue[T]{new})
	if old == nil {
		return common.DefaultValue[T]()
	}
	return old.(typedValue[T]).value
}

func (t *TypedValue[T]) CompareAndSwap(old, new T) bool {
	return t.value.CompareAndSwap(typedValue[T]{old}, typedValue[T]{new}) ||
		// In the edge-case where [atomic.Value.Store] is uninitialized
		// and trying to compare with the zero value of T,
		// then compare-and-swap with the nil any value.
		(any(old) == any(common.DefaultValue[T]()) && t.value.CompareAndSwap(any(nil), typedValue[T]{new}))
}
