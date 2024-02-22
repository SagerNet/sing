package winpowrprof

// modify from https://github.com/golang/go/blob/b634f6fdcbebee23b7da709a243f3db217b64776/src/runtime/os_windows.go#L257

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modpowerprof                                 = windows.NewLazySystemDLL("powrprof.dll")
	procPowerRegisterSuspendResumeNotification   = modpowerprof.NewProc("PowerRegisterSuspendResumeNotification")
	procPowerUnregisterSuspendResumeNotification = modpowerprof.NewProc("PowerUnregisterSuspendResumeNotification")
)

const (
	PBT_APMSUSPEND         uint32 = 4
	PBT_APMRESUMESUSPEND   uint32 = 7
	PBT_APMRESUMEAUTOMATIC uint32 = 18
)

const (
	_DEVICE_NOTIFY_CALLBACK = 2
)

type _DEVICE_NOTIFY_SUBSCRIBE_PARAMETERS struct {
	callback uintptr
	context  uintptr
}

type eventListener struct {
	params _DEVICE_NOTIFY_SUBSCRIBE_PARAMETERS
	handle uintptr
}

func NewEventListener(callback func(event int)) (EventListener, error) {
	if err := procPowerRegisterSuspendResumeNotification.Find(); err != nil {
		return nil, err // Running on Windows 7, where we don't need it anyway.
	}
	if err := procPowerUnregisterSuspendResumeNotification.Find(); err != nil {
		return nil, err // Running on Windows 7, where we don't need it anyway.
	}

	var fn interface{} = func(context uintptr, changeType uint32, setting uintptr) uintptr {
		switch changeType {
		case PBT_APMSUSPEND:
			callback(EVENT_SUSPEND)
		case PBT_APMRESUMESUSPEND:
			callback(EVENT_RESUME)
		case PBT_APMRESUMEAUTOMATIC:
			callback(EVENT_RESUME_AUTOMATIC)
		}
		return 0
	}
	return &eventListener{
		params: _DEVICE_NOTIFY_SUBSCRIBE_PARAMETERS{
			callback: windows.NewCallback(fn),
		},
	}, nil
}

func (l *eventListener) Start() error {
	_, _, errno := syscall.SyscallN(
		procPowerRegisterSuspendResumeNotification.Addr(),
		_DEVICE_NOTIFY_CALLBACK,
		uintptr(unsafe.Pointer(&l.params)),
		uintptr(unsafe.Pointer(&l.handle)),
	)
	if errno != 0 {
		return errno
	}
	return nil
}

func (l *eventListener) Close() error {
	_, _, errno := syscall.SyscallN(procPowerUnregisterSuspendResumeNotification.Addr(), uintptr(unsafe.Pointer(&l.handle)))
	if errno != 0 {
		return errno
	}
	return nil
}
