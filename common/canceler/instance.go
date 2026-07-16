package canceler

import (
	"context"
	"net"
	"os"
	"sync"
	"time"
)

type Instance struct {
	ctx        context.Context
	cancelFunc context.CancelCauseFunc
	timer      *time.Timer
	timeout    time.Duration
	access     sync.Mutex
	closed     bool
}

func New(ctx context.Context, cancelFunc context.CancelCauseFunc, timeout time.Duration) *Instance {
	instance := &Instance{
		ctx:        ctx,
		cancelFunc: cancelFunc,
		timer:      time.NewTimer(timeout),
		timeout:    timeout,
	}
	go instance.wait()
	return instance
}

func (i *Instance) Update() bool {
	i.access.Lock()
	defer i.access.Unlock()
	return i.update()
}

func (i *Instance) update() bool {
	if i.closed {
		return false
	}
	if !i.timer.Stop() {
		return false
	}
	i.timer.Reset(i.timeout)
	return true
}

func (i *Instance) Timeout() time.Duration {
	i.access.Lock()
	defer i.access.Unlock()
	return i.timeout
}

func (i *Instance) SetTimeout(timeout time.Duration) bool {
	i.access.Lock()
	defer i.access.Unlock()
	i.timeout = timeout
	return i.update()
}

func (i *Instance) wait() {
	select {
	case <-i.timer.C:
	case <-i.ctx.Done():
	}
	i.CloseWithError(os.ErrDeadlineExceeded)
}

func (i *Instance) Close() error {
	i.CloseWithError(net.ErrClosed)
	return nil
}

func (i *Instance) CloseWithError(err error) {
	i.access.Lock()
	if i.closed {
		i.access.Unlock()
		return
	}
	i.closed = true
	i.timer.Stop()
	i.access.Unlock()
	i.cancelFunc(err)
}
