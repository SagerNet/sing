package common

import (
	"os"
	"syscall"
)

func TryFileDescriptor(conn any) (uintptr, error) {
	if rawConn, isRaw := conn.(syscall.Conn); isRaw {
		return GetFileDescriptor(rawConn)
	}
	return 0, os.ErrInvalid
}

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
