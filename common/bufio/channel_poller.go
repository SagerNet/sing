package bufio

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"

	N "github.com/sagernet/sing/common/network"
)

type channelDemuxEntry struct {
	channel <-chan *N.PacketBuffer
	stream  *reactorStream
}

type ChannelPoller struct {
	ctx        context.Context
	cancel     context.CancelFunc
	mutex      sync.Mutex
	entries    map[<-chan *N.PacketBuffer]*channelDemuxEntry
	updateChan chan struct{}
	running    bool
	closed     atomic.Bool
	wg         sync.WaitGroup
}

func NewChannelPoller(ctx context.Context) *ChannelPoller {
	ctx, cancel := context.WithCancel(ctx)
	poller := &ChannelPoller{
		ctx:        ctx,
		cancel:     cancel,
		entries:    make(map[<-chan *N.PacketBuffer]*channelDemuxEntry),
		updateChan: make(chan struct{}, 1),
	}
	return poller
}

func (p *ChannelPoller) Add(stream *reactorStream, channel <-chan *N.PacketBuffer) {
	p.mutex.Lock()

	if p.closed.Load() {
		p.mutex.Unlock()
		return
	}

	entry := &channelDemuxEntry{
		channel: channel,
		stream:  stream,
	}
	p.entries[channel] = entry
	if !p.running {
		p.running = true
		p.wg.Add(1)
		go p.run()
	}
	p.mutex.Unlock()
	p.signalUpdate()
}

func (p *ChannelPoller) Remove(channel <-chan *N.PacketBuffer) {
	p.mutex.Lock()
	delete(p.entries, channel)
	p.mutex.Unlock()
	p.signalUpdate()
}

func (p *ChannelPoller) signalUpdate() {
	select {
	case p.updateChan <- struct{}{}:
	default:
	}
}

func (p *ChannelPoller) Close() error {
	p.mutex.Lock()
	p.closed.Store(true)
	p.mutex.Unlock()

	p.cancel()
	p.signalUpdate()
	p.wg.Wait()
	return nil
}

func (p *ChannelPoller) run() {
	defer p.wg.Done()

	for {
		p.mutex.Lock()
		if len(p.entries) == 0 {
			p.running = false
			p.mutex.Unlock()
			return
		}

		cases := make([]reflect.SelectCase, 0, len(p.entries)+2)

		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(p.ctx.Done()),
		})

		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(p.updateChan),
		})

		entryList := make([]*channelDemuxEntry, 0, len(p.entries))
		for _, entry := range p.entries {
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(entry.channel),
			})
			entryList = append(entryList, entry)
		}
		p.mutex.Unlock()

		chosen, recv, recvOK := reflect.Select(cases)

		switch chosen {
		case 0:
			p.mutex.Lock()
			p.running = false
			p.mutex.Unlock()
			return
		case 1:
			continue
		default:
			entry := entryList[chosen-2]
			p.mutex.Lock()
			delete(p.entries, entry.channel)
			p.mutex.Unlock()

			if recvOK {
				packet := recv.Interface().(*N.PacketBuffer)
				go entry.stream.runActiveLoop(packet)
			} else {
				go entry.stream.closeWithError(nil)
			}
		}
	}
}
