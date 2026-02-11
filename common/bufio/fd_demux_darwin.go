//go:build darwin

package bufio

import (
	"context"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

type fdDemuxEntry struct {
	fd     int
	stream *reactorStream
}

type FDDemultiplexer struct {
	ctx      context.Context
	cancel   context.CancelFunc
	kqueueFD int
	mutex    sync.Mutex
	entries  map[int]*fdDemuxEntry
	running  bool
	closed   atomic.Bool
	wg       sync.WaitGroup
	pipeFDs  [2]int
}

func NewFDDemultiplexer(ctx context.Context) (*FDDemultiplexer, error) {
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
	demux := &FDDemultiplexer{
		ctx:      ctx,
		cancel:   cancel,
		kqueueFD: kqueueFD,
		entries:  make(map[int]*fdDemuxEntry),
		pipeFDs:  pipeFDs,
	}
	return demux, nil
}

func (d *FDDemultiplexer) Add(stream *reactorStream, fd int) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.closed.Load() {
		return unix.EINVAL
	}

	_, err := unix.Kevent(d.kqueueFD, []unix.Kevent_t{{
		Ident:  uint64(fd),
		Filter: unix.EVFILT_READ,
		Flags:  unix.EV_ADD,
	}}, nil, nil)
	if err != nil {
		return err
	}

	entry := &fdDemuxEntry{
		fd:     fd,
		stream: stream,
	}
	d.entries[fd] = entry

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

	_, ok := d.entries[fd]
	if !ok {
		return
	}

	unix.Kevent(d.kqueueFD, []unix.Kevent_t{{
		Ident:  uint64(fd),
		Filter: unix.EVFILT_READ,
		Flags:  unix.EV_DELETE,
	}}, nil, nil)
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

	if d.kqueueFD != -1 {
		unix.Close(d.kqueueFD)
		d.kqueueFD = -1
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

	events := make([]unix.Kevent_t, 64)
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

		n, err := unix.Kevent(d.kqueueFD, nil, events, nil)
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
			fd := int(event.Ident)

			if fd == d.pipeFDs[0] {
				unix.Read(d.pipeFDs[0], buffer[:])
				continue
			}

			if event.Flags&unix.EV_ERROR != 0 {
				continue
			}

			d.mutex.Lock()
			entry, ok := d.entries[fd]
			if !ok {
				d.mutex.Unlock()
				continue
			}

			unix.Kevent(d.kqueueFD, []unix.Kevent_t{{
				Ident:  uint64(fd),
				Filter: unix.EVFILT_READ,
				Flags:  unix.EV_DELETE,
			}}, nil, nil)
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
