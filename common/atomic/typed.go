package atomic

import (
	"sync/atomic"

	"github.com/metacubex/sing/common"
)

type TypedValue[T any] atomic.Pointer[T]

func (t *TypedValue[T]) Load() T {
	value := (*atomic.Pointer[T])(t).Load()
	if value == nil {
		return common.DefaultValue[T]()
	}
	return *value
}

func (t *TypedValue[T]) Store(value T) {
	(*atomic.Pointer[T])(t).Store(&value)
}

func (t *TypedValue[T]) Swap(new T) T {
	old := (*atomic.Pointer[T])(t).Swap(&new)
	if old == nil {
		return common.DefaultValue[T]()
	}
	return *old
}
