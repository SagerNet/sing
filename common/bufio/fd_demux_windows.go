//go:build windows

package bufio

import (
	"context"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/sagernet/sing/common/wepoll"

	"golang.org/x/sys/windows"
)

type fdDemuxEntry struct {
	ioStatusBlock  windows.IO_STATUS_BLOCK
	pollInfo       wepoll.AFDPollInfo
	stream         *reactorStream
	fd             int
	handle         windows.Handle
	baseHandle     windows.Handle
	registrationID uint64
	cancelled      bool
	pinner         wepoll.Pinner
}

type FDDemultiplexer struct {
	ctx                 context.Context
	cancel              context.CancelFunc
	iocp                windows.Handle
	afd                 *wepoll.AFD
	mutex               sync.Mutex
	entries             map[int]*fdDemuxEntry
	registrationCounter uint64
	running             bool
	closed              atomic.Bool
	wg                  sync.WaitGroup
}

func NewFDDemultiplexer(ctx context.Context) (*FDDemultiplexer, error) {
	iocp, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 0)
	if err != nil {
		return nil, err
	}

	afd, err := wepoll.NewAFD(iocp, "Go")
	if err != nil {
		windows.CloseHandle(iocp)
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	demux := &FDDemultiplexer{
		ctx:     ctx,
		cancel:  cancel,
		iocp:    iocp,
		afd:     afd,
		entries: make(map[int]*fdDemuxEntry),
	}
	return demux, nil
}

func (d *FDDemultiplexer) Add(stream *reactorStream, fd int) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.closed.Load() {
		return windows.ERROR_INVALID_HANDLE
	}

	handle := windows.Handle(fd)
	baseHandle, err := wepoll.GetBaseSocket(handle)
	if err != nil {
		return err
	}

	d.registrationCounter++
	registrationID := d.registrationCounter

	entry := &fdDemuxEntry{
		stream:         stream,
		fd:             fd,
		handle:         handle,
		baseHandle:     baseHandle,
		registrationID: registrationID,
	}

	entry.pinner.Pin(entry)

	events := uint32(wepoll.AFD_POLL_RECEIVE | wepoll.AFD_POLL_DISCONNECT | wepoll.AFD_POLL_ABORT | wepoll.AFD_POLL_LOCAL_CLOSE)
	err = d.afd.Poll(baseHandle, events, &entry.ioStatusBlock, &entry.pollInfo)
	if err != nil {
		entry.pinner.Unpin()
		return err
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

	entry, ok := d.entries[fd]
	if !ok {
		return
	}

	entry.cancelled = true
	if d.afd != nil {
		d.afd.Cancel(&entry.ioStatusBlock)
	}
}

func (d *FDDemultiplexer) wakeup() {
	windows.PostQueuedCompletionStatus(d.iocp, 0, 0, nil)
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

	for fd, entry := range d.entries {
		entry.pinner.Unpin()
		delete(d.entries, fd)
	}

	if d.afd != nil {
		d.afd.Close()
		d.afd = nil
	}
	if d.iocp != 0 {
		windows.CloseHandle(d.iocp)
		d.iocp = 0
	}
	return nil
}

func (d *FDDemultiplexer) run() {
	defer d.wg.Done()

	completions := make([]wepoll.OverlappedEntry, 64)

	for {
		select {
		case <-d.ctx.Done():
			d.mutex.Lock()
			d.running = false
			d.mutex.Unlock()
			return
		default:
		}

		var numRemoved uint32
		err := wepoll.GetQueuedCompletionStatusEx(d.iocp, &completions[0], 64, &numRemoved, windows.INFINITE, false)
		if err != nil {
			d.mutex.Lock()
			d.running = false
			d.mutex.Unlock()
			return
		}

		for i := uint32(0); i < numRemoved; i++ {
			event := completions[i]

			if event.Overlapped == nil {
				continue
			}

			entry := (*fdDemuxEntry)(unsafe.Pointer(event.Overlapped))

			d.mutex.Lock()

			if d.entries[entry.fd] != entry {
				d.mutex.Unlock()
				continue
			}

			entry.pinner.Unpin()
			delete(d.entries, entry.fd)

			if entry.cancelled {
				d.mutex.Unlock()
				continue
			}

			if uint32(entry.ioStatusBlock.Status) == wepoll.STATUS_CANCELLED {
				d.mutex.Unlock()
				continue
			}

			events := entry.pollInfo.Handles[0].Events
			if events&(wepoll.AFD_POLL_RECEIVE|wepoll.AFD_POLL_DISCONNECT|wepoll.AFD_POLL_ABORT|wepoll.AFD_POLL_LOCAL_CLOSE) == 0 {
				d.mutex.Unlock()
				continue
			}

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
