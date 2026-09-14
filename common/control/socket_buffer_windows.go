package control

import (
	"golang.org/x/sys/windows"
)

func setSocketBuffer(fd uintptr, size int) {
	_ = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_RCVBUF, size)
	_ = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_SNDBUF, size)
}
