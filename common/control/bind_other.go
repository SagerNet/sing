//go:build !(linux || windows || darwin)

package control

import "syscall"

func bindToInterface(conn syscall.RawConn, network string, address string, finder InterfaceFinder, interfaceName string, interfaceIndex int, preferInterfaceName bool) error {
	return nil
}

func unbindFromInterface(conn syscall.RawConn, network string, address string) error {
	return nil
}
