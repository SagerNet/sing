package exceptions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
)

type causeError struct {
	message string
	cause   error
}

func (e *causeError) Error() string {
	if e.cause == nil {
		return e.message
	}
	return e.message + ": " + e.cause.Error()
}

func (e *causeError) Unwrap() error {
	return e.cause
}

type extendedError struct {
	message string
	cause   error
}

func (e *extendedError) Error() string {
	if e.cause == nil {
		return e.message
	}
	return e.cause.Error() + ": " + e.message
}

func (e *extendedError) Unwrap() error {
	return e.cause
}

func New(message ...any) error {
	return errors.New(fmt.Sprint(message...))
}

func Cause(cause error, message ...any) error {
	return &causeError{fmt.Sprint(message...), cause}
}

func Extend(cause error, message ...any) error {
	return &extendedError{fmt.Sprint(message...), cause}
}

type HasInnerError interface {
	Unwrap() error
}

func Unwrap(err error) error {
	for {
		inner, ok := err.(HasInnerError)
		if !ok {
			break
		}
		innerErr := inner.Unwrap()
		if innerErr == nil {
			break
		}
		err = innerErr
	}
	return err
}

func IsCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func IsClosed(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, os.ErrClosed)
}

type TimeoutError interface {
	Timeout() bool
}

func IsTimeout(err error) bool {
	if unwrapErr := errors.Unwrap(err); unwrapErr != nil {
		err = unwrapErr
	}
	if ne, ok := err.(*os.SyscallError); ok {
		err = ne.Err
	}
	if timeoutErr, isTimeoutErr := err.(TimeoutError); isTimeoutErr {
		return timeoutErr.Timeout()
	}
	return false
}

type Handler interface {
	HandleError(err error)
}
