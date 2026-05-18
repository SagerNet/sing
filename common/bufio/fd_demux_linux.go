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
	stream         *reactorStream
}

type FDDemultiplexer struct {
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

func NewFDDemultiplexer(ctx context.Context) (*FDDemultiplexer, error) {
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
	demux := &FDDemultiplexer{
		ctx:              ctx,
		cancel:           cancel,
		epollFD:          epollFD,
		entries:          make(map[int]*fdDemuxEntry),
		registrationToFD: make(map[uint64]int),
		pipeFDs:          pipeFDs,
	}
	return demux, nil
}

func (d *FDDemultiplexer) Add(stream *reactorStream, fd int) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.closed.Load() {
		return unix.EINVAL
	}

	d.registrationCounter++
	registrationID := d.registrationCounter

	event := &unix.EpollEvent{Events: unix.EPOLLIN | unix.EPOLLRDHUP}
	*(*uint64)(unsafe.Pointer(&event.Fd)) = registrationID

	err := unix.EpollCtl(d.epollFD, unix.EPOLL_CTL_ADD, fd, event)
	if err != nil {
		return err
	}

	entry := &fdDemuxEntry{
		fd:             fd,
		registrationID: registrationID,
		stream:         stream,
	}
	d.entries[fd] = entry
	d.registrationToFD[registrationID] = fd

	if !d.running {
		d.running = true
		d.wg.Add(1)
		go d.run()
	}

	return nil
}

func (d *FDDemultiplexer) Remove(fd int) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	entry, ok := d.entries[fd]
	if !ok {
		return
	}

	unix.EpollCtl(d.epollFD, unix.EPOLL_CTL_DEL, fd, nil)
	delete(d.registrationToFD, entry.registrationID)
	delete(d.entries, fd)
}

func (d *FDDemultiplexer) wakeup() {
	unix.Write(d.pipeFDs[1], []byte{0})
}

func (d *FDDemultiplexer) Close() error {
	d.mutex.Lock()
	d.closed.Store(true)
	d.mutex.Unlock()

	d.cancel()
	d.wakeup()
	d.wg.Wait()

	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.epollFD != -1 {
		unix.Close(d.epollFD)
		d.epollFD = -1
	}
	if d.pipeFDs[0] != -1 {
		unix.Close(d.pipeFDs[0])
		unix.Close(d.pipeFDs[1])
		d.pipeFDs[0] = -1
		d.pipeFDs[1] = -1
	}
	return nil
}

func (d *FDDemultiplexer) run() {
	defer d.wg.Done()

	events := make([]unix.EpollEvent, 64)
	var buffer [1]byte

	for {
		select {
		case <-d.ctx.Done():
			d.mutex.Lock()
			d.running = false
			d.mutex.Unlock()
			return
		default:
		}

		n, err := unix.EpollWait(d.epollFD, events, -1)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			d.mutex.Lock()
			d.running = false
			d.mutex.Unlock()
			return
		}

		for i := 0; i < n; i++ {
			event := events[i]
			registrationID := *(*uint64)(unsafe.Pointer(&event.Fd))

			if registrationID == 0 {
				unix.Read(d.pipeFDs[0], buffer[:])
				continue
			}

			if event.Events&(unix.EPOLLIN|unix.EPOLLRDHUP|unix.EPOLLHUP|unix.EPOLLERR) == 0 {
				continue
			}

			d.mutex.Lock()
			fd, ok := d.registrationToFD[registrationID]
			if !ok {
				d.mutex.Unlock()
				continue
			}

			entry := d.entries[fd]
			if entry == nil || entry.registrationID != registrationID {
				d.mutex.Unlock()
				continue
			}

			unix.EpollCtl(d.epollFD, unix.EPOLL_CTL_DEL, fd, nil)
			delete(d.registrationToFD, registrationID)
			delete(d.entries, fd)
			d.mutex.Unlock()

			go entry.stream.runActiveLoop(nil)
		}

		d.mutex.Lock()
		if len(d.entries) == 0 {
			d.running = false
			d.mutex.Unlock()
			return
		}
		d.mutex.Unlock()
	}
}
