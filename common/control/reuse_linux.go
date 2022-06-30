package control

import (
	"syscall"

	"github.com/sagernet/sing/common"
)

func ReuseAddr() Func {
	return func(network, address string, conn syscall.RawConn) error {
		var innerErr error
		err := conn.Control(func(fd uintptr) {
			const SO_REUSEPORT = 0xf
			innerErr = common.AnyError(
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1),
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, SO_REUSEPORT, 1),
			)
		})
		return common.AnyError(innerErr, err)
	}
}
