//go:build !linux

package system

import "github.com/sagernet/sing/common/exceptions"

func TCPFastOpen(fd uintptr) error {
	return exceptions.New("only available on linux")
}
