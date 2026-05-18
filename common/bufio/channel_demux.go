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

type ChannelDemultiplexer struct {
	ctx        context.Context
	cancel     context.CancelFunc
	mutex      sync.Mutex
	entries    map[<-chan *N.PacketBuffer]*channelDemuxEntry
	updateChan chan struct{}
	running    bool
	closed     atomic.Bool
	wg         sync.WaitGroup
}

func NewChannelDemultiplexer(ctx context.Context) *ChannelDemultiplexer {
	ctx, cancel := context.WithCancel(ctx)
	demux := &ChannelDemultiplexer{
		ctx:        ctx,
		cancel:     cancel,
		entries:    make(map[<-chan *N.PacketBuffer]*channelDemuxEntry),
		updateChan: make(chan struct{}, 1),
	}
	return demux
}

func (d *ChannelDemultiplexer) Add(stream *reactorStream, channel <-chan *N.PacketBuffer) {
	d.mutex.Lock()

	if d.closed.Load() {
		d.mutex.Unlock()
		return
	}

	entry := &channelDemuxEntry{
		channel: channel,
		stream:  stream,
	}
	d.entries[channel] = entry
	if !d.running {
		d.running = true
		d.wg.Add(1)
		go d.run()
	}
	d.mutex.Unlock()
	d.signalUpdate()
}

func (d *ChannelDemultiplexer) Remove(channel <-chan *N.PacketBuffer) {
	d.mutex.Lock()
	delete(d.entries, channel)
	d.mutex.Unlock()
	d.signalUpdate()
}

func (d *ChannelDemultiplexer) signalUpdate() {
	select {
	case d.updateChan <- struct{}{}:
	default:
	}
}

func (d *ChannelDemultiplexer) Close() error {
	d.mutex.Lock()
	d.closed.Store(true)
	d.mutex.Unlock()

	d.cancel()
	d.signalUpdate()
	d.wg.Wait()
	return nil
}

func (d *ChannelDemultiplexer) run() {
	defer d.wg.Done()

	for {
		d.mutex.Lock()
		if len(d.entries) == 0 {
			d.running = false
			d.mutex.Unlock()
			return
		}

		cases := make([]reflect.SelectCase, 0, len(d.entries)+2)

		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(d.ctx.Done()),
		})

		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(d.updateChan),
		})

		entryList := make([]*channelDemuxEntry, 0, len(d.entries))
		for _, entry := range d.entries {
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(entry.channel),
			})
			entryList = append(entryList, entry)
		}
		d.mutex.Unlock()

		chosen, recv, recvOK := reflect.Select(cases)

		switch chosen {
		case 0:
			d.mutex.Lock()
			d.running = false
			d.mutex.Unlock()
			return
		case 1:
			continue
		default:
			entry := entryList[chosen-2]
			d.mutex.Lock()
			delete(d.entries, entry.channel)
			d.mutex.Unlock()

			if recvOK {
				packet := recv.Interface().(*N.PacketBuffer)
				go entry.stream.runActiveLoop(packet)
			} else {
				go entry.stream.closeWithError(nil)
			}
		}
	}
}
