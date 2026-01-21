//go:build linux

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
	epollFD             int
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
	epollFD, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}

	var pipeFDs [2]int
	err = unix.Pipe2(pipeFDs[:], unix.O_NONBLOCK|unix.O_CLOEXEC)
	if err != nil {
		unix.Close(epollFD)
		return nil, err
	}

	pipeEvent := &unix.EpollEvent{Events: unix.EPOLLIN}
	*(*uint64)(unsafe.Pointer(&pipeEvent.Fd)) = 0
	err = unix.EpollCtl(epollFD, unix.EPOLL_CTL_ADD, pipeFDs[0], pipeEvent)
	if err != nil {
		unix.Close(pipeFDs[0])
		unix.Close(pipeFDs[1])
		unix.Close(epollFD)
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	poller := &FDPoller{
		ctx:              ctx,
		cancel:           cancel,
		epollFD:          epollFD,
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

	event := &unix.EpollEvent{Events: unix.EPOLLIN | unix.EPOLLRDHUP}
	*(*uint64)(unsafe.Pointer(&event.Fd)) = registrationID

	err := unix.EpollCtl(p.epollFD, unix.EPOLL_CTL_ADD, fd, event)
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

	unix.EpollCtl(p.epollFD, unix.EPOLL_CTL_DEL, fd, nil)
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

	if p.epollFD != -1 {
		unix.Close(p.epollFD)
		p.epollFD = -1
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

	events := make([]unix.EpollEvent, 64)
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

		n, err := unix.EpollWait(p.epollFD, events, -1)
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
			registrationID := *(*uint64)(unsafe.Pointer(&event.Fd))

			if registrationID == 0 {
				unix.Read(p.pipeFDs[0], buffer[:])
				continue
			}

			if event.Events&(unix.EPOLLIN|unix.EPOLLRDHUP|unix.EPOLLHUP|unix.EPOLLERR) == 0 {
				continue
			}

			p.mutex.Lock()
			fd, ok := p.registrationToFD[registrationID]
			if !ok {
				p.mutex.Unlock()
				continue
			}

			entry := p.entries[fd]
			if entry == nil || entry.registrationID != registrationID {
				p.mutex.Unlock()
				continue
			}

			unix.EpollCtl(p.epollFD, unix.EPOLL_CTL_DEL, fd, nil)
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
