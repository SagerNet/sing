package control

import (
	"encoding/binary"
	"os"
	"syscall"
	"unsafe"

	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
)

func bindToInterface(conn syscall.RawConn, network string, address string, finder InterfaceFinder, interfaceName string, interfaceIndex int, preferInterfaceName bool) error {
	return Raw(conn, func(fd uintptr) error {
		if interfaceIndex == -1 {
			if finder == nil {
				return os.ErrInvalid
			}
			iif, err := finder.ByName(interfaceName)
			if err != nil {
				return err
			}
			interfaceIndex = iif.Index
		}
		handle := syscall.Handle(fd)
		if M.ParseSocksaddr(address).AddrString() == "" {
			// IP_UNICAST_IF fails with WSAEINVAL on IPV6_V6ONLY sockets, and IPV6_UNICAST_IF
			// fails with WSAEADDRNOTAVAIL on AF_INET sockets and when the interface has IPv6
			// disabled.
			err4 := bind4(handle, interfaceIndex)
			err6 := bind6(handle, interfaceIndex)
			if err4 != nil && err6 != nil {
				return E.Errors(err4, err6)
			}
			return nil
		}
		switch network {
		case "tcp4", "udp4", "ip4":
			return bind4(handle, interfaceIndex)
		default:
			return bind6(handle, interfaceIndex)
		}
	})
}

func unbindFromInterface(conn syscall.RawConn, network string, address string) error {
	return bindToInterface(conn, network, address, nil, "", 0, false)
}

const (
	IP_UNICAST_IF   = 31
	IPV6_UNICAST_IF = 31
)

func bind4(handle syscall.Handle, ifaceIdx int) error {
	var bytes [4]byte
	binary.BigEndian.PutUint32(bytes[:], uint32(ifaceIdx))
	idx := *(*uint32)(unsafe.Pointer(&bytes[0]))
	return syscall.SetsockoptInt(handle, syscall.IPPROTO_IP, IP_UNICAST_IF, int(idx))
}

func bind6(handle syscall.Handle, ifaceIdx int) error {
	return syscall.SetsockoptInt(handle, syscall.IPPROTO_IPV6, IPV6_UNICAST_IF, ifaceIdx)
}
