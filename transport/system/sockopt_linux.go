package system

import (
	"syscall"
)

const (
	TCP_FASTOPEN         = 23
	TCP_FASTOPEN_CONNECT = 30
)

func TCPFastOpen(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, TCP_FASTOPEN_CONNECT, 1)
}
