//go:build go1.21

package common

import "sync"

func OnceFunc(f func()) func() {
	return sync.OnceFunc(f)
}

func OnceValue[T any](f func() T) func() T {
	return sync.OnceValue(f)
}

func OnceValues[T1, T2 any](f func() (T1, T2)) func() (T1, T2) {
	return sync.OnceValues(f)
}
