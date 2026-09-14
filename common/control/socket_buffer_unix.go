//go:build unix && !linux

package control

import (
	"golang.org/x/sys/unix"
)

func setSocketBuffer(fd uintptr, size int) {
	_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_RCVBUF, size)
	_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_SNDBUF, size)
}
