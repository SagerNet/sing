package batch

import (
	"context"
	"sync"

	"github.com/metacubex/sing/common"
	E "github.com/metacubex/sing/common/exceptions"
)

type Option[T any] func(b *Batch[T])

type Result[T any] struct {
	Value T
	Err   error
}

type Error struct {
	Key string
	Err error
}

func (e *Error) Error() string {
	return E.Cause(e.Err, e.Key).Error()
}

func WithConcurrencyNum[T any](n int) Option[T] {
	return func(b *Batch[T]) {
		q := make(chan struct{}, n)
		for i := 0; i < n; i++ {
			q <- struct{}{}
		}
		b.queue = q
	}
}

// Batch similar to errgroup, but can control the maximum number of concurrent
type Batch[T any] struct {
	result map[string]Result[T]
	queue  chan struct{}
	wg     sync.WaitGroup
	mux    sync.Mutex
	err    *Error
	once   sync.Once
	cancel common.ContextCancelCauseFunc
}

func (b *Batch[T]) Go(key string, fn func() (T, error)) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		if b.queue != nil {
			<-b.queue
			defer func() {
				b.queue <- struct{}{}
			}()
		}

		value, err := fn()
		if err != nil {
			b.once.Do(func() {
				b.err = &Error{key, err}
				if b.cancel != nil {
					b.cancel(b.err)
				}
			})
		}

		ret := Result[T]{value, err}
		b.mux.Lock()
		defer b.mux.Unlock()
		b.result[key] = ret
	}()
}

func (b *Batch[T]) Wait() *Error {
	b.wg.Wait()
	if b.cancel != nil {
		b.cancel(nil)
	}
	return b.err
}

func (b *Batch[T]) WaitAndGetResult() (map[string]Result[T], *Error) {
	err := b.Wait()
	return b.Result(), err
}

func (b *Batch[T]) Result() map[string]Result[T] {
	b.mux.Lock()
	defer b.mux.Unlock()
	copy := map[string]Result[T]{}
	for k, v := range b.result {
		copy[k] = v
	}
	return copy
}

func New[T any](ctx context.Context, opts ...Option[T]) (*Batch[T], context.Context) {
	ctx, cancel := common.ContextWithCancelCause(ctx)

	b := &Batch[T]{
		result: map[string]Result[T]{},
	}

	for _, o := range opts {
		o(b)
	}

	b.cancel = cancel
	return b, ctx
}
