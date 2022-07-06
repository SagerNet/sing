//nolint:errorlint
package exceptions

import (
	"context"
	"errors"
	"io"
	"net"
	"os"

	"github.com/sagernet/sing/common"
	F "github.com/sagernet/sing/common/format"
)

type Handler interface {
	NewError(ctx context.Context, err error)
}

type MultiError interface {
	UnwrapMulti() []error
}

func New(message ...any) error {
	return errors.New(F.ToString(message...))
}

func Cause(cause error, message ...any) error {
	return &causeError{F.ToString(message...), cause}
}

func Extend(cause error, message ...any) error {
	return &extendedError{F.ToString(message...), cause}
}

func Errors(errors ...error) error {
	errors = common.FilterNotNil(errors)
	switch len(errors) {
	case 0:
		return nil
	case 1:
		return errors[0]
	}
	return &multiError{
		errors: errors,
	}
}

func IsCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func IsClosed(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, os.ErrClosed)
}
