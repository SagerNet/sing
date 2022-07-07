package control

import (
	"syscall"

	E "github.com/sagernet/sing/common/exceptions"
)

func ReuseAddr() Func {
	return func(network, address string, conn syscall.RawConn) error {
		var innerErr error
		err := conn.Control(func(fd uintptr) {
			const SO_REUSEPORT = 0xf
			innerErr = E.Errors(
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1),
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, SO_REUSEPORT, 1),
			)
		})
		return E.Errors(innerErr, err)
	}
}
