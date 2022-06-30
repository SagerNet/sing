package control

import (
	"syscall"
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
