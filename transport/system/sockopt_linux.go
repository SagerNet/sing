package system

import (
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	TCP_FASTOPEN         = 23
	TCP_FASTOPEN_CONNECT = 30
)

func TCPFastOpen(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_TCP, TCP_FASTOPEN_CONNECT, 1)
}

func TProxy(fd uintptr, isIPv6 bool) error {
	err := syscall.SetsockoptInt(int(fd), syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
	if err != nil {
		return err
	}
	if isIPv6 {
		err = syscall.SetsockoptInt(int(fd), syscall.SOL_IPV6, unix.IPV6_TRANSPARENT, 1)
	}
	return err
}

func TProxyUDP(fd uintptr, isIPv6 bool) error {
	err := syscall.SetsockoptInt(int(fd), syscall.SOL_IPV6, unix.IPV6_RECVORIGDSTADDR, 1)
	if err != nil {
		return err
	}
	return syscall.SetsockoptInt(int(fd), syscall.SOL_IP, syscall.IP_RECVORIGDSTADDR, 1)
}
