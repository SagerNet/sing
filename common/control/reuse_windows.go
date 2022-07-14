package control

import (
	"syscall"

	E "github.com/sagernet/sing/common/exceptions"
)

func ReuseAddr() Func {
	return func(network, address string, conn syscall.RawConn) error {
		var innerErr error
		err := conn.Control(func(fd uintptr) {
			innerErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
		})
		return E.Errors(innerErr, err)
	}
}
