package control

import (
	"syscall"

	E "github.com/sagernet/sing/common/exceptions"
)

func NewBindManager() BindManager {
	return nil
}

func BindToInterface(manager BindManager, interfaceName string) Func {
	return func(network, address string, conn syscall.RawConn) error {
		var innerErr error
		err := conn.Control(func(fd uintptr) {
			innerErr = syscall.BindToDevice(int(fd), interfaceName)
		})
		return E.Errors(innerErr, err)
	}
}

func BindToInterfaceFunc(manager BindManager, interfaceNameFunc func() string) Func {
	return func(network, address string, conn syscall.RawConn) error {
		interfaceName := interfaceNameFunc()
		if interfaceName == "" {
			return nil
		}
		var innerErr error
		err := conn.Control(func(fd uintptr) {
			innerErr = syscall.BindToDevice(int(fd), interfaceName)
		})
		return E.Errors(innerErr, err)
	}
}

func BindToInterfaceIndexFunc(interfaceIndexFunc func() int) Func {
	return nil
}
