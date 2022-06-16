package common

import (
	"syscall"
)

func GetFileDescriptor(conn syscall.Conn) (uintptr, error) {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var rawFd uintptr
	err = rawConn.Control(func(fd uintptr) {
		rawFd = fd
	})
	return rawFd, err
}
