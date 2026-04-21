//go:build windows

package wepoll

import (
	"math"
	"unsafe"

	"golang.org/x/sys/windows"
)

type AFD struct {
	handle windows.Handle
}

func NewAFD(iocp windows.Handle, name string) (*AFD, error) {
	deviceName := `\Device\Afd\` + name
	deviceNameUTF16, err := windows.UTF16FromString(deviceName)
	if err != nil {
		return nil, err
	}

	unicodeString := UnicodeString{
		Length:        uint16(len(deviceName) * 2),
		MaximumLength: uint16(len(deviceName) * 2),
		Buffer:        &deviceNameUTF16[0],
	}

	objectAttributes := ObjectAttributes{
		Length:     uint32(unsafe.Sizeof(ObjectAttributes{})),
		ObjectName: &unicodeString,
		Attributes: OBJ_CASE_INSENSITIVE,
	}

	var handle windows.Handle
	var ioStatusBlock windows.IO_STATUS_BLOCK

	err = NtCreateFile(
		&handle,
		windows.SYNCHRONIZE,
		&objectAttributes,
		&ioStatusBlock,
		nil,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		FILE_OPEN,
		0,
		0,
		0,
	)
	if err != nil {
		return nil, err
	}

	_, err = windows.CreateIoCompletionPort(handle, iocp, 0, 0)
	if err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}

	err = windows.SetFileCompletionNotificationModes(handle, windows.FILE_SKIP_SET_EVENT_ON_HANDLE)
	if err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}

	return &AFD{handle: handle}, nil
}

func (a *AFD) Poll(baseSocket windows.Handle, events uint32, iosb *windows.IO_STATUS_BLOCK, pollInfo *AFDPollInfo) error {
	pollInfo.Timeout = math.MaxInt64
	pollInfo.NumberOfHandles = 1
	pollInfo.Exclusive = 0
	pollInfo.Handles[0].Handle = baseSocket
	pollInfo.Handles[0].Events = events
	pollInfo.Handles[0].Status = 0

	size := uint32(unsafe.Sizeof(*pollInfo))

	err := NtDeviceIoControlFile(
		a.handle,
		0,
		0,
		uintptr(unsafe.Pointer(iosb)),
		iosb,
		IOCTL_AFD_POLL,
		unsafe.Pointer(pollInfo),
		size,
		unsafe.Pointer(pollInfo),
		size,
	)
	if err != nil {
		if ntstatus, ok := err.(windows.NTStatus); ok {
			if uint32(ntstatus) == STATUS_PENDING {
				return nil
			}
		}
		return err
	}
	return nil
}

func (a *AFD) Cancel(ioStatusBlock *windows.IO_STATUS_BLOCK) error {
	if uint32(ioStatusBlock.Status) != STATUS_PENDING {
		return nil
	}
	var cancelIOStatusBlock windows.IO_STATUS_BLOCK
	err := NtCancelIoFileEx(a.handle, ioStatusBlock, &cancelIOStatusBlock)
	if err != nil {
		if ntstatus, ok := err.(windows.NTStatus); ok {
			if uint32(ntstatus) == STATUS_CANCELLED || uint32(ntstatus) == STATUS_NOT_FOUND {
				return nil
			}
		}
		return err
	}
	return nil
}

func (a *AFD) Close() error {
	return windows.CloseHandle(a.handle)
}
