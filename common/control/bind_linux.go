package control

import (
	"syscall"
)

func NewBindManager() BindManager {
	return nil
}

func BindToInterface(manager BindManager, interfaceName string) Func {
	return func(network, address string, conn syscall.RawConn) error {
		return Raw(conn, func(fd uintptr) error {
			return syscall.BindToDevice(int(fd), interfaceName)
		})
	}
}

func BindToInterfaceFunc(manager BindManager, interfaceNameFunc func(network, address string) string) Func {
	return func(network, address string, conn syscall.RawConn) error {
		interfaceName := interfaceNameFunc(network, address)
		if interfaceName == "" {
			return nil
		}
		return Raw(conn, func(fd uintptr) error {
			return syscall.BindToDevice(int(fd), interfaceName)
		})
	}
}

func BindToInterfaceIndexFunc(interfaceIndexFunc func(network, address string) int) Func {
	return nil
}
