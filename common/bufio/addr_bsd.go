//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package bufio

import (
	"encoding/binary"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/unix"
)

func ToSockaddr(destination netip.AddrPort) (name unsafe.Pointer, nameLen uint32) {
	if destination.Addr().Is4() {
		sa := unix.RawSockaddrInet4{
			Len:    unix.SizeofSockaddrInet4,
			Family: unix.AF_INET,
			Addr:   destination.Addr().As4(),
		}
		binary.BigEndian.PutUint16((*[2]byte)(unsafe.Pointer(&sa.Port))[:], destination.Port())
		name = unsafe.Pointer(&sa)
		nameLen = unix.SizeofSockaddrInet4
	} else {
		sa := unix.RawSockaddrInet6{
			Len:    unix.SizeofSockaddrInet6,
			Family: unix.AF_INET6,
			Addr:   destination.Addr().As16(),
		}
		binary.BigEndian.PutUint16((*[2]byte)(unsafe.Pointer(&sa.Port))[:], destination.Port())
		name = unsafe.Pointer(&sa)
		nameLen = unix.SizeofSockaddrInet6
	}
	return
}
