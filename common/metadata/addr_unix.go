//go:build unix

package metadata

import (
	"encoding/binary"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/unix"
)

func AddrPortFromSockaddr(sa unix.Sockaddr) netip.AddrPort {
	switch addr := sa.(type) {
	case *unix.SockaddrInet4:
		return netip.AddrPortFrom(netip.AddrFrom4(addr.Addr), uint16(addr.Port))
	case *unix.SockaddrInet6:
		return netip.AddrPortFrom(netip.AddrFrom16(addr.Addr), uint16(addr.Port))
	default:
		return netip.AddrPort{}
	}
}

func AddrPortToSockaddr(addrPort netip.AddrPort) unix.Sockaddr {
	if addrPort.Addr().Is4() {
		return &unix.SockaddrInet4{
			Port: int(addrPort.Port()),
			Addr: addrPort.Addr().As4(),
		}
	} else {
		return &unix.SockaddrInet6{
			Port: int(addrPort.Port()),
			Addr: addrPort.Addr().As16(),
		}
	}
}

func AddrPortFromRawSockaddr(sa *unix.RawSockaddr) netip.AddrPort {
	switch sa.Family {
	case unix.AF_INET:
		sa4 := (*unix.RawSockaddrInet4)(unsafe.Pointer(sa))
		port := binary.BigEndian.Uint16((*[2]byte)(unsafe.Pointer(&sa4.Port))[:])
		return netip.AddrPortFrom(netip.AddrFrom4(sa4.Addr), port)
	case unix.AF_INET6:
		sa6 := (*unix.RawSockaddrInet6)(unsafe.Pointer(sa))
		port := binary.BigEndian.Uint16((*[2]byte)(unsafe.Pointer(&sa6.Port))[:])
		return netip.AddrPortFrom(netip.AddrFrom16(sa6.Addr), port)
	default:
		return netip.AddrPort{}
	}
}

func SocksaddrFromRawSockaddrAny(sa *unix.RawSockaddrAny) Socksaddr {
	return SocksaddrFromNetIP(AddrPortFromRawSockaddr(&sa.Addr)).Unwrap()
}

func AddrPortToRawSockaddrAny(name *unix.RawSockaddrAny, addrPort netip.AddrPort, forceInet6 bool) uint32 {
	rawName, nameLen := AddrPortToRawSockaddr(addrPort, forceInet6)
	*name = unix.RawSockaddrAny{}
	if nameLen == unix.SizeofSockaddrInet4 {
		*(*unix.RawSockaddrInet4)(unsafe.Pointer(name)) = *(*unix.RawSockaddrInet4)(rawName)
	} else {
		*(*unix.RawSockaddrInet6)(unsafe.Pointer(name)) = *(*unix.RawSockaddrInet6)(rawName)
	}
	return nameLen
}
