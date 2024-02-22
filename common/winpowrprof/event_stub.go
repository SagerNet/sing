//go:build !windows

package winpowrprof

import (
	"os"
)

func NewEventListener(callback func(event int)) (EventListener, error) {
	return nil, os.ErrInvalid
}
