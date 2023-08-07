package pause

type Manager interface {
	DevicePause()
	DeviceWake()
	DevicePauseChan() <-chan struct{}
	NetworkPause()
	NetworkWake()
	NetworkPauseChan() <-chan struct{}
	IsPaused() bool
	WaitActive()
}
