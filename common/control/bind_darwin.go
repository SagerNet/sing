package control

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func bindToInterface(conn syscall.RawConn, network string, address string, finder InterfaceFinder, interfaceName string, interfaceIndex int, preferInterfaceName bool) error {
	return Raw(conn, func(fd uintptr) error {
		var err error
		if interfaceIndex == -1 {
			if finder == nil {
				return os.ErrInvalid
			}
			interfaceIndex, err = finder.InterfaceIndexByName(interfaceName)
			if err != nil {
				return err
			}
		}
		switch network {
		case "tcp6", "udp6":
			return unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_BOUND_IF, interfaceIndex)
		default:
			return unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_BOUND_IF, interfaceIndex)
		}
	})
}
