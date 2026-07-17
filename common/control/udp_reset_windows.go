package control

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func DisableUDPNetReset() Func {
	return func(network, address string, conn syscall.RawConn) error {
		return Raw(conn, func(fileDescriptor uintptr) error {
			optionValue := uint32(0)
			var bytesReturned uint32
			return windows.WSAIoctl(windows.Handle(fileDescriptor), windows.SIO_UDP_NETRESET, (*byte)(unsafe.Pointer(&optionValue)), uint32(unsafe.Sizeof(optionValue)), nil, 0, &bytesReturned, nil, 0)
		})
	}
}
