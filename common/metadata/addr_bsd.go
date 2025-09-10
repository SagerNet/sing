//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package metadata

import (
	"encoding/binary"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/unix"
)

func AddrPortToRawSockaddr(addrPort netip.AddrPort) (name unsafe.Pointer, nameLen uint32) {
	if addrPort.Addr().Is4() {
		var sa unix.RawSockaddrInet4
		sa.Len = unix.SizeofSockaddrInet4
		sa.Family = unix.AF_INET
		sa.Addr = addrPort.Addr().As4()
		binary.BigEndian.PutUint16((*[2]byte)(unsafe.Pointer(&sa.Port))[:], addrPort.Port())
		return unsafe.Pointer(&sa), unix.SizeofSockaddrInet4
	} else {
		var sa unix.RawSockaddrInet6
		sa.Len = unix.SizeofSockaddrInet6
		sa.Family = unix.AF_INET6
		sa.Addr = addrPort.Addr().As16()
		binary.BigEndian.PutUint16((*[2]byte)(unsafe.Pointer(&sa.Port))[:], addrPort.Port())
		return unsafe.Pointer(&sa), unix.SizeofSockaddrInet6
	}
}
