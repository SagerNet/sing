package system

import "syscall"

var ControlFunc func(fd uintptr) error

func Control(conn syscall.Conn) error {
	if ControlFunc == nil {
		return nil
	}
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	return ControlRaw(rawConn)
}

func ControlRaw(conn syscall.RawConn) error {
	if ControlFunc == nil {
		return nil
	}
	var rawFd uintptr
	err := conn.Control(func(fd uintptr) {
		rawFd = fd
	})
	if err != nil {
		return err
	}
	return ControlFunc(rawFd)
}
