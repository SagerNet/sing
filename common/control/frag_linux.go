package control

import (
	"os"
	"syscall"

	N "github.com/metacubex/sing/common/network"

	"golang.org/x/sys/unix"
)

func DisableUDPFragment() Func {
	return func(network, address string, conn syscall.RawConn) error {
		if N.NetworkName(network) != N.NetworkUDP {
			return nil
		}
		return Raw(conn, func(fd uintptr) error {
			if network == "udp" || network == "udp4" {
				err := unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_MTU_DISCOVER, unix.IP_PMTUDISC_DO)
				if err != nil {
					return os.NewSyscallError("SETSOCKOPT IP_MTU_DISCOVER IP_PMTUDISC_DO", err)
				}
			}
			if network == "udp" || network == "udp6" {
				err := unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_MTU_DISCOVER, unix.IP_PMTUDISC_DO)
				if err != nil {
					return os.NewSyscallError("SETSOCKOPT IPV6_MTU_DISCOVER IP_PMTUDISC_DO", err)
				}
			}
			return nil
		})
	}
}
