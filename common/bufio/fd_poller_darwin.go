//go:build darwin

package bufio

import (
	"context"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"
)

type fdDemuxEntry struct {
	fd             int
	registrationID uint64
	handler        FDHandler
}

type FDPoller struct {
	ctx                 context.Context
	cancel              context.CancelFunc
	kqueueFD            int
	mutex               sync.Mutex
	entries             map[int]*fdDemuxEntry
	registrationCounter uint64
	registrationToFD    map[uint64]int
	running             bool
	closed              atomic.Bool
	wg                  sync.WaitGroup
	pipeFDs             [2]int
}

func NewFDPoller(ctx context.Context) (*FDPoller, error) {
	kqueueFD, err := unix.Kqueue()
	if err != nil {
		return nil, err
	}

	var pipeFDs [2]int
	err = unix.Pipe(pipeFDs[:])
	if err != nil {
		unix.Close(kqueueFD)
		return nil, err
	}

	err = unix.SetNonblock(pipeFDs[0], true)
	if err != nil {
		unix.Close(pipeFDs[0])
		unix.Close(pipeFDs[1])
		unix.Close(kqueueFD)
		return nil, err
	}
	err = unix.SetNonblock(pipeFDs[1], true)
	if err != nil {
		unix.Close(pipeFDs[0])
		unix.Close(pipeFDs[1])
		unix.Close(kqueueFD)
		return nil, err
	}

	_, err = unix.Kevent(kqueueFD, []unix.Kevent_t{{
		Ident:  uint64(pipeFDs[0]),
		Filter: unix.EVFILT_READ,
		Flags:  unix.EV_ADD,
	}}, nil, nil)
	if err != nil {
		unix.Close(pipeFDs[0])
		unix.Close(pipeFDs[1])
		unix.Close(kqueueFD)
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	poller := &FDPoller{
		ctx:              ctx,
		cancel:           cancel,
		kqueueFD:         kqueueFD,
		entries:          make(map[int]*fdDemuxEntry),
		registrationToFD: make(map[uint64]int),
		pipeFDs:          pipeFDs,
	}
	return poller, nil
}

func (p *FDPoller) Add(handler FDHandler, fd int) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed.Load() {
		return unix.EINVAL
	}

	p.registrationCounter++
	registrationID := p.registrationCounter

	_, err := unix.Kevent(p.kqueueFD, []unix.Kevent_t{{
		Ident:  uint64(fd),
		Filter: unix.EVFILT_READ,
		Flags:  unix.EV_ADD | unix.EV_ONESHOT,
		Udata:  (*byte)(unsafe.Pointer(uintptr(registrationID))),
	}}, nil, nil)
	if err != nil {
		return err
	}

	entry := &fdDemuxEntry{
		fd:             fd,
		registrationID: registrationID,
		handler:        handler,
	}
	p.entries[fd] = entry
	p.registrationToFD[registrationID] = fd

	if !p.running {
		p.running = true
		p.wg.Add(1)
		go p.run()
	}

	return nil
}

func (p *FDPoller) Remove(fd int) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	entry, ok := p.entries[fd]
	if !ok {
		return
	}

	unix.Kevent(p.kqueueFD, []unix.Kevent_t{{
		Ident:  uint64(fd),
		Filter: unix.EVFILT_READ,
		Flags:  unix.EV_DELETE,
	}}, nil, nil)
	delete(p.registrationToFD, entry.registrationID)
	delete(p.entries, fd)
}

func (p *FDPoller) wakeup() {
	unix.Write(p.pipeFDs[1], []byte{0})
}

func (p *FDPoller) Close() error {
	p.mutex.Lock()
	p.closed.Store(true)
	p.mutex.Unlock()

	p.cancel()
	p.wakeup()
	p.wg.Wait()

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.kqueueFD != -1 {
		unix.Close(p.kqueueFD)
		p.kqueueFD = -1
	}
	if p.pipeFDs[0] != -1 {
		unix.Close(p.pipeFDs[0])
		unix.Close(p.pipeFDs[1])
		p.pipeFDs[0] = -1
		p.pipeFDs[1] = -1
	}
	return nil
}

func (p *FDPoller) run() {
	defer p.wg.Done()

	events := make([]unix.Kevent_t, 64)
	var buffer [1]byte

	for {
		select {
		case <-p.ctx.Done():
			p.mutex.Lock()
			p.running = false
			p.mutex.Unlock()
			return
		default:
		}

		n, err := unix.Kevent(p.kqueueFD, nil, events, nil)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			p.mutex.Lock()
			p.running = false
			p.mutex.Unlock()
			return
		}

		for i := 0; i < n; i++ {
			event := events[i]
			fd := int(event.Ident)

			if fd == p.pipeFDs[0] {
				unix.Read(p.pipeFDs[0], buffer[:])
				continue
			}

			if event.Flags&unix.EV_ERROR != 0 {
				continue
			}

			registrationID := uint64(uintptr(unsafe.Pointer(event.Udata)))

			p.mutex.Lock()
			mappedFD, ok := p.registrationToFD[registrationID]
			if !ok || mappedFD != fd {
				p.mutex.Unlock()
				continue
			}

			entry := p.entries[fd]
			if entry == nil || entry.registrationID != registrationID {
				p.mutex.Unlock()
				continue
			}

			delete(p.registrationToFD, registrationID)
			delete(p.entries, fd)
			p.mutex.Unlock()

			go entry.handler.HandleFDEvent()
		}

		p.mutex.Lock()
		if len(p.entries) == 0 {
			p.running = false
			p.mutex.Unlock()
			return
		}
		p.mutex.Unlock()
	}
}
