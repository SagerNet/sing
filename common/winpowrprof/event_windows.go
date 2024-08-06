package winpowrprof

// modify from https://github.com/golang/go/blob/b634f6fdcbebee23b7da709a243f3db217b64776/src/runtime/os_windows.go#L257

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/sagernet/sing/common/x/list"

	"golang.org/x/sys/windows"
)

type powerEventListener struct {
	element *list.Element[EventCallback]
}

func NewEventListener(callback EventCallback) (EventListener, error) {
	err := initCallback()
	if err != nil {
		return nil, err
	}
	access.Lock()
	defer access.Unlock()
	return &powerEventListener{
		element: callbackList.PushBack(callback),
	}, nil
}

func (l *powerEventListener) Start() error {
	access.Lock()
	defer access.Unlock()
	if handle != 0 {
		return nil
	}
	return startListener()
}

func (l *powerEventListener) Close() error {
	access.Lock()
	defer access.Unlock()
	if l.element != nil {
		callbackList.Remove(l.element)
	}
	if callbackList.Len() > 0 {
		return nil
	}
	return closeListener()
}

var (
	modpowerprof                                 = windows.NewLazySystemDLL("powrprof.dll")
	procPowerRegisterSuspendResumeNotification   = modpowerprof.NewProc("PowerRegisterSuspendResumeNotification")
	procPowerUnregisterSuspendResumeNotification = modpowerprof.NewProc("PowerUnregisterSuspendResumeNotification")
)

var (
	access           sync.Mutex
	callbackList     list.List[EventCallback]
	initCallbackOnce sync.Once
	rawCallback      uintptr
	handle           uintptr
)

func initCallback() error {
	err := procPowerRegisterSuspendResumeNotification.Find()
	if err != nil {
		return err // Running on Windows 7, where we don't need it anyway.
	}
	err = procPowerUnregisterSuspendResumeNotification.Find()
	if err != nil {
		return err // Running on Windows 7, where we don't need it anyway.
	}
	initCallbackOnce.Do(func() {
		rawCallback = windows.NewCallback(func(context uintptr, changeType uint32, setting uintptr) uintptr {
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
			access.Lock()
			callbacks := callbackList.Array()
			access.Unlock()
			for _, callback := range callbacks {
				callback(event)
			}
			return 0
		})
	})
	return nil
}

func startListener() error {
	type DEVICE_NOTIFY_SUBSCRIBE_PARAMETERS struct {
		callback uintptr
		context  uintptr
	}
	const DEVICE_NOTIFY_CALLBACK = 2
	params := DEVICE_NOTIFY_SUBSCRIBE_PARAMETERS{
		callback: rawCallback,
	}
	_, _, errno := syscall.SyscallN(
		procPowerRegisterSuspendResumeNotification.Addr(),
		DEVICE_NOTIFY_CALLBACK,
		uintptr(unsafe.Pointer(&params)),
		uintptr(unsafe.Pointer(&handle)),
	)
	if errno != 0 {
		return errno
	}
	return nil
}

func closeListener() error {
	if handle == 0 {
		return nil
	}
	_, _, errno := syscall.SyscallN(procPowerUnregisterSuspendResumeNotification.Addr(), uintptr(unsafe.Pointer(&handle)))
	if errno != 0 {
		return errno
	}
	handle = 0
	return nil
}
