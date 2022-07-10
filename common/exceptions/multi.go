package exceptions

import (
	"errors"
	"strings"

	"github.com/sagernet/sing/common"
	F "github.com/sagernet/sing/common/format"
)

type multiError struct {
	errors []error
}

func (e *multiError) Error() string {
	return "multi error: (" + strings.Join(F.MapToString(e.errors), " | ") + ")"
}

func (e *multiError) Unwrap() error {
	return e.errors[0]
}

func (e *multiError) UnwrapMulti() []error {
	return e.errors
}

func (e *multiError) Is(err error) bool {
	return common.Any(e.errors, func(it error) bool {
		return errors.Is(it, err)
	})
}
