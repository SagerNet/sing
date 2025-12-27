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
	handler        FDHandler
	fd             int
	handle         windows.Handle
	baseHandle     windows.Handle
	registrationID uint64
	cancelled      bool
	unpinned       bool
	pinner         wepoll.Pinner
}

type FDPoller struct {
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

func NewFDPoller(ctx context.Context) (*FDPoller, error) {
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
	poller := &FDPoller{
		ctx:     ctx,
		cancel:  cancel,
		iocp:    iocp,
		afd:     afd,
		entries: make(map[int]*fdDemuxEntry),
	}
	return poller, nil
}

func (p *FDPoller) Add(handler FDHandler, fd int) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed.Load() {
		return windows.ERROR_INVALID_HANDLE
	}

	handle := windows.Handle(fd)
	baseHandle, err := wepoll.GetBaseSocket(handle)
	if err != nil {
		return err
	}

	p.registrationCounter++
	registrationID := p.registrationCounter

	entry := &fdDemuxEntry{
		handler:        handler,
		fd:             fd,
		handle:         handle,
		baseHandle:     baseHandle,
		registrationID: registrationID,
	}

	entry.pinner.Pin(entry)

	events := uint32(wepoll.AFD_POLL_RECEIVE | wepoll.AFD_POLL_DISCONNECT | wepoll.AFD_POLL_ABORT | wepoll.AFD_POLL_LOCAL_CLOSE)
	err = p.afd.Poll(baseHandle, events, &entry.ioStatusBlock, &entry.pollInfo)
	if err != nil {
		entry.pinner.Unpin()
		return err
	}

	p.entries[fd] = entry

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

	entry.cancelled = true
	if p.afd != nil {
		p.afd.Cancel(&entry.ioStatusBlock)
	}
}

func (p *FDPoller) wakeup() {
	windows.PostQueuedCompletionStatus(p.iocp, 0, 0, nil)
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

	for fd, entry := range p.entries {
		if !entry.unpinned {
			entry.unpinned = true
			entry.pinner.Unpin()
		}
		delete(p.entries, fd)
	}

	if p.afd != nil {
		p.afd.Close()
		p.afd = nil
	}
	if p.iocp != 0 {
		windows.CloseHandle(p.iocp)
		p.iocp = 0
	}
	return nil
}

func (p *FDPoller) drainCompletions(completions []wepoll.OverlappedEntry) {
	for {
		var numRemoved uint32
		err := wepoll.GetQueuedCompletionStatusEx(p.iocp, &completions[0], uint32(len(completions)), &numRemoved, 0, false)
		if err != nil || numRemoved == 0 {
			break
		}

		for i := uint32(0); i < numRemoved; i++ {
			event := completions[i]
			if event.Overlapped == nil {
				continue
			}

			entry := (*fdDemuxEntry)(unsafe.Pointer(event.Overlapped))
			p.mutex.Lock()
			if p.entries[entry.fd] == entry && !entry.unpinned {
				entry.unpinned = true
				entry.pinner.Unpin()
			}
			delete(p.entries, entry.fd)
			p.mutex.Unlock()
		}
	}
}

func (p *FDPoller) run() {
	defer p.wg.Done()

	completions := make([]wepoll.OverlappedEntry, 64)

	for {
		select {
		case <-p.ctx.Done():
			p.drainCompletions(completions)
			p.mutex.Lock()
			p.running = false
			p.mutex.Unlock()
			return
		default:
		}

		var numRemoved uint32
		err := wepoll.GetQueuedCompletionStatusEx(p.iocp, &completions[0], 64, &numRemoved, windows.INFINITE, false)
		if err != nil {
			p.mutex.Lock()
			p.running = false
			p.mutex.Unlock()
			return
		}

		for i := uint32(0); i < numRemoved; i++ {
			event := completions[i]

			if event.Overlapped == nil {
				continue
			}

			entry := (*fdDemuxEntry)(unsafe.Pointer(event.Overlapped))

			p.mutex.Lock()

			if p.entries[entry.fd] != entry {
				p.mutex.Unlock()
				continue
			}

			if !entry.unpinned {
				entry.unpinned = true
				entry.pinner.Unpin()
			}
			delete(p.entries, entry.fd)

			if entry.cancelled {
				p.mutex.Unlock()
				continue
			}

			if uint32(entry.ioStatusBlock.Status) == wepoll.STATUS_CANCELLED {
				p.mutex.Unlock()
				continue
			}

			events := entry.pollInfo.Handles[0].Events
			if events&(wepoll.AFD_POLL_RECEIVE|wepoll.AFD_POLL_DISCONNECT|wepoll.AFD_POLL_ABORT|wepoll.AFD_POLL_LOCAL_CLOSE) == 0 {
				p.mutex.Unlock()
				continue
			}

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
