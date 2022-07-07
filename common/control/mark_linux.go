package control

import (
	"syscall"

	E "github.com/sagernet/sing/common/exceptions"
)

func RoutingMark(mark int) Func {
	return func(network, address string, conn syscall.RawConn) error {
		var innerErr error
		err := conn.Control(func(fd uintptr) {
			innerErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, mark)
		})
		return E.Errors(innerErr, err)
	}
}
