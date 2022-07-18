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

func (e *multiError) UnwrapMulti() []error {
	return e.errors
}

func IsMulti(err error, targetList ...error) bool {
	for _, target := range targetList {
		if errors.Is(err, target) {
			return true
		}
	}
	err = Unwrap(err)
	multiErr, isMulti := err.(MultiError)
	if !isMulti {
		return false
	}
	return common.All(multiErr.UnwrapMulti(), func(it error) bool {
		return IsMulti(it, targetList...)
	})
}
