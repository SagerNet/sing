package winpowrprof

// modify from https://github.com/golang/go/blob/b634f6fdcbebee23b7da709a243f3db217b64776/src/runtime/os_windows.go#L257

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modpowerprof                                 = windows.NewLazySystemDLL("powrprof.dll")
	procPowerRegisterSuspendResumeNotification   = modpowerprof.NewProc("PowerRegisterSuspendResumeNotification")
	procPowerUnregisterSuspendResumeNotification = modpowerprof.NewProc("PowerUnregisterSuspendResumeNotification")
)

type eventListener struct {
	callback uintptr
	handle   uintptr
}

func NewEventListener(callback func(event int)) (EventListener, error) {
	if err := procPowerRegisterSuspendResumeNotification.Find(); err != nil {
		return nil, err // Running on Windows 7, where we don't need it anyway.
	}
	if err := procPowerUnregisterSuspendResumeNotification.Find(); err != nil {
		return nil, err // Running on Windows 7, where we don't need it anyway.
	}

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
	var listener eventListener
	listener.callback = windows.NewCallback(fn)
	params := _DEVICE_NOTIFY_SUBSCRIBE_PARAMETERS{
		callback: listener.callback,
	}
	_, _, err := procPowerRegisterSuspendResumeNotification.Call(
		_DEVICE_NOTIFY_CALLBACK,
		uintptr(unsafe.Pointer(&params)),
		uintptr(unsafe.Pointer(&listener.handle)),
	)
	if err != nil {
		return nil, err
	}
	return &listener, nil
}

func (l *eventListener) Close() error {
	_, _, err := procPowerUnregisterSuspendResumeNotification.Call(uintptr(unsafe.Pointer(&l.handle)))
	return err
}
