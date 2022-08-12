//go:build !windows

package metadata

import (
	"syscall"
	"unsafe"
)

func (ap Socksaddr) Sockaddr() (name unsafe.Pointer, nameLen uint32) {
	if ap.IsFqdn() {
		panic("bad sockaddr")
	} else if ap.IsIPv4() {
		rsa4 := syscall.RawSockaddrInet4{
			Family: syscall.AF_INET,
			Port:   ap.Port,
			Addr:   ap.Addr.As4(),
		}
		name = unsafe.Pointer(&rsa4)
		nameLen = syscall.SizeofSockaddrInet4
	} else {
		rsa6 := syscall.RawSockaddrInet6{
			Family: syscall.AF_INET6,
			Port:   ap.Port,
			Addr:   ap.Addr.As16(),
		}
		name = unsafe.Pointer(&rsa6)
		nameLen = syscall.SizeofSockaddrInet6
	}
	return
}
