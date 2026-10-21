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
		switch network {
		case "tcp4", "udp4", "ip4":
			return bind4(handle, interfaceIndex)
		}
		ipv6Only, err := syscall.GetsockoptInt(handle, syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY)
		if err != nil {
			return err
		}
		if ipv6Only != 0 {
			return bind6(handle, interfaceIndex)
		}
		// IPV6_UNICAST_IF fails with WSAEADDRNOTAVAIL when the interface has IPv6 disabled.
		err4 := bind4(handle, interfaceIndex)
		err6 := bind6(handle, interfaceIndex)
		socketAddress := M.ParseSocksaddr(address).Addr.Unmap()
		if socketAddress.IsValid() && !socketAddress.IsUnspecified() {
			if socketAddress.Is4() {
				return err4
			}
			return err6
		}
		if err4 != nil && err6 != nil {
			return E.Errors(err4, err6)
		}
		return nil
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
