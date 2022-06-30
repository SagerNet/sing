package control

import (
	"syscall"

	"github.com/sagernet/sing/common"
)

func BindToInterface(interfaceName string) Func {
	return func(network, address string, conn syscall.RawConn) error {
		var innerErr error
		err := conn.Control(func(fd uintptr) {
			innerErr = syscall.BindToDevice(int(fd), interfaceName)
		})
		return common.AnyError(innerErr, err)
	}
}
