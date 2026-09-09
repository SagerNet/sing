package control

import (
	"os"

	"golang.org/x/sys/unix"
)

func setSocketBuffer(fd uintptr, size int) error {
	err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_RCVBUFFORCE, size)
	if err != nil {
		err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_RCVBUF, size)
	}
	if err != nil {
		return os.NewSyscallError("SETSOCKOPT SO_RCVBUF", err)
	}
	err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_SNDBUFFORCE, size)
	if err != nil {
		err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_SNDBUF, size)
	}
	if err != nil {
		return os.NewSyscallError("SETSOCKOPT SO_SNDBUF", err)
	}
	return nil
}
