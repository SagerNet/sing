package exceptions

import (
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
)

type Exception interface {
	error
	Cause() error
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

func (e exception) Unwrap() error {
	return e.cause
}

func (e exception) Is(err error) bool {
	return e == err || errors.Is(e.cause, err)
}

func New(message ...any) error {
	return errors.New(fmt.Sprint(message...))
}

func Cause(cause error, message string) Exception {
	return exception{message, cause}
}

func CauseF(cause error, message ...any) Exception {
	return exception{fmt.Sprint(message), cause}
}

func IsClosed(err error) bool {
	return IsTimeout(err) || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, syscall.EPIPE)
}

func IsTimeout(err error) bool {
	if unwrapErr := errors.Unwrap(err); unwrapErr != nil {
		err = unwrapErr
	}
	if opErr, isOpErr := err.(*net.OpError); isOpErr {
		return opErr.Timeout()
	}
	return false
}
