package pause

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/sagernet/sing/common/x/list"
	"github.com/sagernet/sing/service"
)

type defaultManager struct {
	ctx           context.Context
	access        sync.Mutex
	devicePaused  atomic.Bool
	networkPaused atomic.Bool
	active        atomic.Pointer[chan struct{}]
	callbacks     list.List[Callback]
}

func WithDefaultManager(ctx context.Context) context.Context {
	if service.FromContext[Manager](ctx) != nil {
		return ctx
	}
	active := make(chan struct{})
	close(active)
	manager := &defaultManager{
		ctx: ctx,
	}
	manager.active.Store(&active)
	return service.ContextWith[Manager](ctx, manager)
}

func (d *defaultManager) DevicePause() {
	d.access.Lock()
	defer d.access.Unlock()
	if d.devicePaused.Load() {
		return
	}
	d.pauseLocked()
	d.devicePaused.Store(true)
	d.emit(EventDevicePaused)
}

func (d *defaultManager) DeviceWake() {
	d.access.Lock()
	defer d.access.Unlock()
	if !d.devicePaused.Swap(false) {
		return
	}
	d.wakeLocked()
	d.emit(EventDeviceWake)
}

func (d *defaultManager) NetworkPause() {
	d.access.Lock()
	defer d.access.Unlock()
	if d.networkPaused.Load() {
		return
	}
	d.pauseLocked()
	d.networkPaused.Store(true)
	d.emit(EventNetworkPause)
}

func (d *defaultManager) NetworkWake() {
	d.access.Lock()
	defer d.access.Unlock()
	if !d.networkPaused.Swap(false) {
		return
	}
	d.wakeLocked()
	d.emit(EventNetworkWake)
}

func (d *defaultManager) pauseLocked() {
	select {
	case <-*d.active.Load():
		active := make(chan struct{})
		d.active.Store(&active)
	default:
	}
}

func (d *defaultManager) wakeLocked() {
	if !d.devicePaused.Load() && !d.networkPaused.Load() {
		close(*d.active.Load())
	}
}

func (d *defaultManager) RegisterCallback(callback Callback) *list.Element[Callback] {
	d.access.Lock()
	defer d.access.Unlock()
	return d.callbacks.PushBack(callback)
}

func (d *defaultManager) UnregisterCallback(element *list.Element[Callback]) {
	d.access.Lock()
	defer d.access.Unlock()
	d.callbacks.Remove(element)
}

func (d *defaultManager) IsDevicePaused() bool {
	return d.devicePaused.Load()
}

func (d *defaultManager) IsNetworkPaused() bool {
	return d.networkPaused.Load()
}

func (d *defaultManager) IsPaused() bool {
	select {
	case <-*d.active.Load():
		return false
	default:
		return true
	}
}

func (d *defaultManager) WaitActive() {
	d.WaitActiveContext(d.ctx)
}

func (d *defaultManager) WaitActiveContext(ctx context.Context) {
	select {
	case <-*d.active.Load():
	case <-ctx.Done():
	case <-d.ctx.Done():
	}
}

func (d *defaultManager) emit(event int) {
	for element := d.callbacks.Front(); element != nil; element = element.Next() {
		element.Value(event)
	}
}
