package common

import (
	"context"
	"io"
	"runtime"
	"unsafe"
)

func Any[T any](array []T, block func(it T) bool) bool {
	for _, it := range array {
		if block(it) {
			return true
		}
	}
	return false
}

func All[T any](array []T, block func(it T) bool) bool {
	for _, it := range array {
		if !block(it) {
			return false
		}
	}
	return true
}

func Contains[T comparable](arr []T, target T) bool {
	for i := range arr {
		if target == arr[i] {
			return true
		}
	}
	return false
}

func Map[T any, N any](arr []T, block func(it T) N) []N {
	retArr := make([]N, 0, len(arr))
	for index := range arr {
		retArr = append(retArr, block(arr[index]))
	}
	return retArr
}

func MapIndexed[T any, N any](arr []T, block func(index int, it T) N) []N {
	retArr := make([]N, 0, len(arr))
	for index := range arr {
		retArr = append(retArr, block(index, arr[index]))
	}
	return retArr
}

func Filter[T any](arr []T, block func(it T) bool) []T {
	var retArr []T
	for _, it := range arr {
		if block(it) {
			retArr = append(retArr, it)
		}
	}
	return retArr
}

func Find[T any](arr []T, block func(it T) bool) T {
	for _, it := range arr {
		if block(it) {
			return it
		}
	}
	var defaultValue T
	return defaultValue
}

//go:norace
func Dup[T any](obj T) T {
	if UnsafeBuffer {
		//nolint:staticcheck
		//goland:noinspection GoVetUnsafePointer
		return *(*T)(unsafe.Pointer(uintptr(unsafe.Pointer(&obj)) ^ 0))
	} else {
		return obj
	}
}

func KeepAlive(obj any) {
	if UnsafeBuffer {
		runtime.KeepAlive(obj)
	}
}

func Uniq[T comparable](arr []T) []T {
	result := make([]T, 0, len(arr))
	seen := make(map[T]struct{}, len(arr))

	for _, item := range arr {
		if _, ok := seen[item]; ok {
			continue
		}

		seen[item] = struct{}{}
		result = append(result, item)
	}

	return result
}

func FilterIsInstance[T any, N any](arr []T, block func(it T) (N, bool)) []N {
	var retArr []N
	for _, it := range arr {
		if n, isN := block(it); isN {
			retArr = append(retArr, n)
		}
	}
	return retArr
}

func Done(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func Error(_ any, err error) error {
	return err
}

func Must(errs ...error) {
	for _, err := range errs {
		if err != nil {
			panic(err)
		}
	}
}

func Must1(_ any, err error) {
	if err != nil {
		panic(err)
	}
}

func Must2(_, _ any, err error) {
	if err != nil {
		panic(err)
	}
}

func AnyError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func PtrOrNil[T any](ptr *T) any {
	if ptr == nil {
		return nil
	}
	return ptr
}

func PtrValueOrDefault[T any](ptr *T) T {
	if ptr == nil {
		var defaultValue T
		return defaultValue
	}
	return *ptr
}

func IsEmpty[T comparable](obj T) bool {
	var defaultValue T
	return obj == defaultValue
}

func Close(closers ...any) error {
	var retErr error
	for _, closer := range closers {
		if closer == nil {
			continue
		}
		switch c := closer.(type) {
		case io.Closer:
			err := c.Close()
			if err != nil {
				retErr = err
			}
			continue
		case WithUpstream:
			err := Close(c.Upstream())
			if err != nil {
				retErr = err
			}
		}
	}
	return retErr
}
