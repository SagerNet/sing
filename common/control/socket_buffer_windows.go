package control

import (
	"os"

	"golang.org/x/sys/windows"
)

func setSocketBuffer(fd uintptr, size int) error {
	err := windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_RCVBUF, size)
	if err != nil {
		return os.NewSyscallError("SETSOCKOPT SO_RCVBUF", err)
	}
	err = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_SNDBUF, size)
	if err != nil {
		return os.NewSyscallError("SETSOCKOPT SO_SNDBUF", err)
	}
	return nil
}
