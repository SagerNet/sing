//go:build windows

package wepoll

import "golang.org/x/sys/windows"

const (
	IOCTL_AFD_POLL = 0x00012024

	AFD_POLL_RECEIVE           = 0x0001
	AFD_POLL_RECEIVE_EXPEDITED = 0x0002
	AFD_POLL_SEND              = 0x0004
	AFD_POLL_DISCONNECT        = 0x0008
	AFD_POLL_ABORT             = 0x0010
	AFD_POLL_LOCAL_CLOSE       = 0x0020
	AFD_POLL_ACCEPT            = 0x0080
	AFD_POLL_CONNECT_FAIL      = 0x0100

	SIO_BASE_HANDLE     = 0x48000022
	SIO_BSP_HANDLE_POLL = 0x4800001D

	STATUS_PENDING   = 0x00000103
	STATUS_CANCELLED = 0xC0000120
	STATUS_NOT_FOUND = 0xC0000225

	FILE_OPEN = 0x00000001

	OBJ_CASE_INSENSITIVE = 0x00000040
)

type AFDPollHandleInfo struct {
	Handle windows.Handle
	Events uint32
	Status uint32
}

type AFDPollInfo struct {
	Timeout         int64
	NumberOfHandles uint32
	Exclusive       uint32
	Handles         [1]AFDPollHandleInfo
}

type OverlappedEntry struct {
	CompletionKey            uintptr
	Overlapped               *windows.Overlapped
	Internal                 uintptr
	NumberOfBytesTransferred uint32
}

type UnicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

type ObjectAttributes struct {
	Length                   uint32
	RootDirectory            windows.Handle
	ObjectName               *UnicodeString
	Attributes               uint32
	SecurityDescriptor       uintptr
	SecurityQualityOfService uintptr
}
