package control

import (
	"syscall"

	E "github.com/sagernet/sing/common/exceptions"
)

type Func = func(network, address string, conn syscall.RawConn) error

func Append(oldFunc Func, newFunc Func) Func {
	if oldFunc == nil {
		return newFunc
	} else if newFunc == nil {
		return oldFunc
	}
	return func(network, address string, conn syscall.RawConn) error {
		if err := oldFunc(network, address, conn); err != nil {
			return err
		}
		return newFunc(network, address, conn)
	}
}

func Control(conn syscall.RawConn, block func(fd uintptr) error) error {
	var innerErr error
	err := conn.Control(func(fd uintptr) {
		innerErr = block(fd)
	})
	return E.Errors(innerErr, err)
}
