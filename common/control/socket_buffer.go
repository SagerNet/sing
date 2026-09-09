package control

import (
	"syscall"

	N "github.com/sagernet/sing/common/network"
)

func UDPSocketBuffer(size int) Func {
	return func(network, address string, conn syscall.RawConn) error {
		if N.NetworkName(network) != N.NetworkUDP {
			return nil
		}
		return Raw(conn, func(fd uintptr) error {
			return setSocketBuffer(fd, size)
		})
	}
}
