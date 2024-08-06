package winpowrprof

// modify from https://github.com/golang/go/blob/b634f6fdcbebee23b7da709a243f3db217b64776/src/runtime/os_windows.go#L257

import (
	"syscall"
	"unsafe"

	"github.com/sagernet/sing/common"

	"golang.org/x/sys/windows"
)

var (
	modpowerprof                                 = windows.NewLazySystemDLL("powrprof.dll")
	procPowerRegisterSuspendResumeNotification   = modpowerprof.NewProc("PowerRegisterSuspendResumeNotification")
	procPowerUnregisterSuspendResumeNotification = modpowerprof.NewProc("PowerUnregisterSuspendResumeNotification")
)

var suspendResumeNotificationCallback = common.OnceValue(func() uintptr {
	return windows.NewCallback(func(context *EventCallback, changeType uint32, setting uintptr) uintptr {
		callback := *context
		const (
			PBT_APMSUSPEND         uint32 = 4
			PBT_APMRESUMESUSPEND   uint32 = 7
			PBT_APMRESUMEAUTOMATIC uint32 = 18
		)
		var event int
		switch changeType {
		case PBT_APMSUSPEND:
			event = EVENT_SUSPEND
		case PBT_APMRESUMESUSPEND:
			event = EVENT_RESUME
		case PBT_APMRESUMEAUTOMATIC:
			event = EVENT_RESUME_AUTOMATIC
		default:
			return 0
		}
		callback(event)
		return 0
	})
})

type powerEventListener struct {
	callback EventCallback
	handle   uintptr
}

func NewEventListener(callback EventCallback) (EventListener, error) {
	err := procPowerRegisterSuspendResumeNotification.Find()
	if err != nil {
		return nil, err // Running on Windows 7, where we don't need it anyway.
	}
	err = procPowerUnregisterSuspendResumeNotification.Find()
	if err != nil {
		return nil, err // Running on Windows 7, where we don't need it anyway.
	}
	return &powerEventListener{
		callback: callback,
	}, nil
}

func (l *powerEventListener) Start() error {
	type DEVICE_NOTIFY_SUBSCRIBE_PARAMETERS struct {
		callback uintptr
		context  unsafe.Pointer
	}
	const DEVICE_NOTIFY_CALLBACK = 2
	params := DEVICE_NOTIFY_SUBSCRIBE_PARAMETERS{
		callback: suspendResumeNotificationCallback(),
		context:  unsafe.Pointer(&l.callback),
	}
	_, _, errno := syscall.SyscallN(
		procPowerRegisterSuspendResumeNotification.Addr(),
		DEVICE_NOTIFY_CALLBACK,
		uintptr(unsafe.Pointer(&params)),
		uintptr(unsafe.Pointer(&l.handle)),
	)
	if errno != 0 {
		return errno
	}
	return nil
}

func (l *powerEventListener) Close() error {
	_, _, errno := syscall.SyscallN(procPowerUnregisterSuspendResumeNotification.Addr(), uintptr(unsafe.Pointer(&l.handle)))
	if errno != 0 {
		return errno
	}
	return nil
}
