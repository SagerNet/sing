package control

import (
	"syscall"

	E "github.com/sagernet/sing/common/exceptions"
)

func BindToInterface(interfaceName string) Func {
	return func(network, address string, conn syscall.RawConn) error {
		var innerErr error
		err := conn.Control(func(fd uintptr) {
			innerErr = syscall.BindToDevice(int(fd), interfaceName)
		})
		return E.Errors(innerErr, err)
	}
}
