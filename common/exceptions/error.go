package exceptions

import (
	"errors"
	"fmt"
)

type Exception interface {
	error
	Cause() error
}

type SuppressedException interface {
	error
	Suppressed() error
}

type exception struct {
	message string
	cause   error
}

func (e exception) Error() string {
	if e.cause == nil {
		return e.message
	}
	return e.message + ": " + e.cause.Error()
}

func (e exception) Cause() error {
	return e.cause
}

func New(message ...any) error {
	return errors.New(fmt.Sprint(message...))
}

func Cause(cause error, message ...any) Exception {
	return &exception{fmt.Sprint(message...), cause}
}
