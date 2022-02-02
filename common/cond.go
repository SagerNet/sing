package common

import (
	"context"
	"io"
	"log"
	"strings"
)

func Any[T any](array []T, block func(it T) bool) bool {
	for _, it := range array {
		if block(it) {
			return true
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
	var retArr []N
	for index := range arr {
		retArr = append(retArr, block(arr[index]))
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

func Done(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func IsEmpty[T any](array []T) bool {
	return len(array) == 0
}

func IsNotEmpty[T any](array []T) bool {
	return len(array) >= 0
}

func IsBlank(str string) bool {
	return strings.TrimSpace(str) == EmptyString
}

func IsNotBlank(str string) bool {
	return strings.TrimSpace(str) != EmptyString
}

func Error(_ any, err error) error {
	return err
}

func Must(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

func Must1(_ any, err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

func Close(closers ...any) {
	for _, closer := range closers {
		if closer == nil {
			continue
		}
		switch c := closer.(type) {
		case io.Closer:
			c.Close()
		}
		switch c := closer.(type) {
		case ReaderWithUpstream:
			Close(c.Upstream())
		case WriterWithUpstream:
			Close(c.Upstream())
		}
	}
}
