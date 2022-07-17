package control

import (
	"syscall"
)

func NewBindManager() BindManager {
	return nil
}

func BindToInterface(manager BindManager, interfaceName string) Func {
	return func(network, address string, conn syscall.RawConn) error {
		return Control(conn, func(fd uintptr) error {
			return syscall.BindToDevice(int(fd), interfaceName)
		})
	}
}

func BindToInterfaceFunc(manager BindManager, interfaceNameFunc func() string) Func {
	return func(network, address string, conn syscall.RawConn) error {
		interfaceName := interfaceNameFunc()
		if interfaceName == "" {
			return nil
		}
		return Control(conn, func(fd uintptr) error {
			return syscall.BindToDevice(int(fd), interfaceName)
		})
	}
}

func BindToInterfaceIndexFunc(interfaceIndexFunc func() int) Func {
	return nil
}
