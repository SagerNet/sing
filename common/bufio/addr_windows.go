package bufio

import (
	"encoding/binary"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

func ToSockaddr(destination netip.AddrPort) (name unsafe.Pointer, nameLen int32) {
	if destination.Addr().Is4() {
		sa := windows.RawSockaddrInet4{
			Family: windows.AF_INET,
			Addr:   destination.Addr().As4(),
		}
		binary.BigEndian.PutUint16((*[2]byte)(unsafe.Pointer(&sa.Port))[:], destination.Port())
		name = unsafe.Pointer(&sa)
		nameLen = int32(unsafe.Sizeof(sa))
	} else {
		sa := windows.RawSockaddrInet6{
			Family: windows.AF_INET6,
			Addr:   destination.Addr().As16(),
		}
		binary.BigEndian.PutUint16((*[2]byte)(unsafe.Pointer(&sa.Port))[:], destination.Port())
		name = unsafe.Pointer(&sa)
		nameLen = int32(unsafe.Sizeof(sa))
	}
	return
}
