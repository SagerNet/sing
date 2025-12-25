//go:build windows

package wepoll

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func GetBaseSocket(socket windows.Handle) (windows.Handle, error) {
	var baseSocket windows.Handle
	var bytesReturned uint32

	for {
		err := windows.WSAIoctl(
			socket,
			SIO_BASE_HANDLE,
			nil,
			0,
			(*byte)(unsafe.Pointer(&baseSocket)),
			uint32(unsafe.Sizeof(baseSocket)),
			&bytesReturned,
			nil,
			0,
		)
		if err != nil {
			err = windows.WSAIoctl(
				socket,
				SIO_BSP_HANDLE_POLL,
				nil,
				0,
				(*byte)(unsafe.Pointer(&baseSocket)),
				uint32(unsafe.Sizeof(baseSocket)),
				&bytesReturned,
				nil,
				0,
			)
			if err != nil {
				return socket, nil
			}
		}

		if baseSocket == socket {
			return baseSocket, nil
		}
		socket = baseSocket
	}
}
