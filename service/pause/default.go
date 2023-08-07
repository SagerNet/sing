package pause

import (
	"context"
	"sync"
)

type defaultManager struct {
	ctx          context.Context
	access       sync.Mutex
	devicePause  chan struct{}
	networkPause chan struct{}
}

func NewDefaultManager(ctx context.Context) Manager {
	devicePauseChan := make(chan struct{})
	networkPauseChan := make(chan struct{})
	close(devicePauseChan)
	close(networkPauseChan)
	return &defaultManager{
		ctx:          ctx,
		devicePause:  devicePauseChan,
		networkPause: networkPauseChan,
	}
}

func (d *defaultManager) DevicePause() {
	d.access.Lock()
	defer d.access.Unlock()
	select {
	case <-d.devicePause:
		d.devicePause = make(chan struct{})
	default:
	}
}

func (d *defaultManager) DeviceWake() {
	d.access.Lock()
	defer d.access.Unlock()
	select {
	case <-d.devicePause:
	default:
		close(d.devicePause)
	}
}

func (d *defaultManager) DevicePauseChan() <-chan struct{} {
	return d.devicePause
}

func (d *defaultManager) NetworkPause() {
	d.access.Lock()
	defer d.access.Unlock()
	select {
	case <-d.networkPause:
		d.networkPause = make(chan struct{})
	default:
	}
}

func (d *defaultManager) NetworkWake() {
	d.access.Lock()
	defer d.access.Unlock()
	select {
	case <-d.networkPause:
	default:
		close(d.networkPause)
	}
}

func (d *defaultManager) NetworkPauseChan() <-chan struct{} {
	return d.networkPause
}

func (d *defaultManager) IsPaused() bool {
	select {
	case <-d.devicePause:
	default:
		return true
	}

	select {
	case <-d.networkPause:
	default:
		return true
	}

	return false
}

func (d *defaultManager) WaitActive() {
	select {
	case <-d.devicePause:
	case <-d.ctx.Done():
	}

	select {
	case <-d.networkPause:
	case <-d.ctx.Done():
	}
}
