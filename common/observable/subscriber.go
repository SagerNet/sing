package observable

import "time"

type Subscription[T any] <-chan T

type Subscriber[T any] struct {
	buffer chan T
	done   chan struct{}
}

func (s *Subscriber[T]) Emit(item T) {
	select {
	case <-s.done:
		return
	default:
	}
	select {
	case s.buffer <- item:
	default:
	}
}

func (s *Subscriber[T]) Close() error {
	close(s.done)
	go func() {
		time.Sleep(time.Second)
		for {
			_, loaded := <-s.buffer
			if !loaded {
				break
			}
		}
	}()
	return nil
}

func (s *Subscriber[T]) Subscription() (subscription Subscription[T], done <-chan struct{}) {
	return s.buffer, s.done
}

func NewSubscriber[T any](size int) *Subscriber[T] {
	return &Subscriber[T]{
		buffer: make(chan T, size),
		done:   make(chan struct{}),
	}
}
