package bufio

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

const (
	batchReadTimeout = 250 * time.Millisecond
)

const (
	stateIdle   int32 = 0
	stateActive int32 = 1
	stateClosed int32 = 2
)

type PacketReactor struct {
	ctx          context.Context
	cancel       context.CancelFunc
	channelDemux *ChannelDemultiplexer
	fdDemux      *FDDemultiplexer
	fdDemuxOnce  sync.Once
	fdDemuxErr   error
}

func NewPacketReactor(ctx context.Context) *PacketReactor {
	ctx, cancel := context.WithCancel(ctx)
	return &PacketReactor{
		ctx:          ctx,
		cancel:       cancel,
		channelDemux: NewChannelDemultiplexer(ctx),
	}
}

func (r *PacketReactor) getFDDemultiplexer() (*FDDemultiplexer, error) {
	r.fdDemuxOnce.Do(func() {
		r.fdDemux, r.fdDemuxErr = NewFDDemultiplexer(r.ctx)
	})
	return r.fdDemux, r.fdDemuxErr
}

func (r *PacketReactor) Close() error {
	r.cancel()
	var errs []error
	if r.channelDemux != nil {
		errs = append(errs, r.channelDemux.Close())
	}
	if r.fdDemux != nil {
		errs = append(errs, r.fdDemux.Close())
	}
	return E.Errors(errs...)
}

type reactorConnection struct {
	ctx      context.Context
	cancel   context.CancelFunc
	reactor  *PacketReactor
	onClose  N.CloseHandlerFunc
	upload   *reactorStream
	download *reactorStream

	closeOnce sync.Once
	done      chan struct{}
	err       error
}

type reactorStream struct {
	connection *reactorConnection

	source       N.PacketReader
	destination  N.PacketWriter
	originSource N.PacketReader

	notifier      N.ReadNotifier
	options       N.ReadWaitOptions
	readWaiter    N.PacketReadWaiter
	readCounters  []N.CountFunc
	writeCounters []N.CountFunc

	state atomic.Int32
}

func (r *PacketReactor) Copy(ctx context.Context, source N.PacketConn, destination N.PacketConn, onClose N.CloseHandlerFunc) {
	ctx, cancel := context.WithCancel(ctx)
	conn := &reactorConnection{
		ctx:     ctx,
		cancel:  cancel,
		reactor: r,
		onClose: onClose,
		done:    make(chan struct{}),
	}

	conn.upload = r.prepareStream(conn, source, destination)
	select {
	case <-conn.done:
		return
	default:
	}

	conn.download = r.prepareStream(conn, destination, source)
	select {
	case <-conn.done:
		return
	default:
	}

	r.registerStream(conn.upload)
	r.registerStream(conn.download)
}

func (r *PacketReactor) prepareStream(conn *reactorConnection, source N.PacketReader, destination N.PacketWriter) *reactorStream {
	stream := &reactorStream{
		connection:   conn,
		source:       source,
		destination:  destination,
		originSource: source,
	}

	for {
		source, stream.readCounters = N.UnwrapCountPacketReader(source, stream.readCounters)
		destination, stream.writeCounters = N.UnwrapCountPacketWriter(destination, stream.writeCounters)
		if cachedReader, isCached := source.(N.CachedPacketReader); isCached {
			packet := cachedReader.ReadCachedPacket()
			if packet != nil {
				dataLen := packet.Buffer.Len()
				err := destination.WritePacket(packet.Buffer, packet.Destination)
				N.PutPacketBuffer(packet)
				if err != nil {
					conn.closeWithError(err)
					return stream
				}
				for _, counter := range stream.readCounters {
					counter(int64(dataLen))
				}
				for _, counter := range stream.writeCounters {
					counter(int64(dataLen))
				}
				continue
			}
		}
		break
	}
	stream.source = source
	stream.destination = destination

	stream.options = N.NewReadWaitOptions(source, destination)

	stream.readWaiter, _ = CreatePacketReadWaiter(source)
	if stream.readWaiter != nil {
		stream.readWaiter.InitializeReadWaiter(stream.options)
	}

	if notifierSource, ok := source.(N.ReadNotifierSource); ok {
		stream.notifier = notifierSource.CreateReadNotifier()
	}

	return stream
}

func (r *PacketReactor) registerStream(stream *reactorStream) {
	if stream.notifier == nil {
		go stream.runLegacyCopy()
		return
	}

	switch notifier := stream.notifier.(type) {
	case *N.ChannelNotifier:
		r.channelDemux.Add(stream, notifier.Channel)
	case *N.FileDescriptorNotifier:
		fdDemux, err := r.getFDDemultiplexer()
		if err != nil {
			go stream.runLegacyCopy()
			return
		}
		err = fdDemux.Add(stream, notifier.FD)
		if err != nil {
			go stream.runLegacyCopy()
		}
	default:
		go stream.runLegacyCopy()
	}
}

func (s *reactorStream) runActiveLoop(firstPacket *N.PacketBuffer) {
	if s.source == nil {
		if firstPacket != nil {
			firstPacket.Buffer.Release()
			N.PutPacketBuffer(firstPacket)
		}
		return
	}
	if !s.state.CompareAndSwap(stateIdle, stateActive) {
		if firstPacket != nil {
			firstPacket.Buffer.Release()
			N.PutPacketBuffer(firstPacket)
		}
		return
	}

	notFirstTime := false

	if firstPacket != nil {
		err := s.writePacketWithCounters(firstPacket)
		if err != nil {
			s.closeWithError(err)
			return
		}
		notFirstTime = true
	}

	for {
		if s.state.Load() == stateClosed {
			return
		}

		if setter, ok := s.source.(interface{ SetReadDeadline(time.Time) error }); ok {
			setter.SetReadDeadline(time.Now().Add(batchReadTimeout))
		}

		var (
			buffer      *N.PacketBuffer
			destination M.Socksaddr
			err         error
		)

		if s.readWaiter != nil {
			var readBuffer *buf.Buffer
			readBuffer, destination, err = s.readWaiter.WaitReadPacket()
			if readBuffer != nil {
				buffer = N.NewPacketBuffer()
				buffer.Buffer = readBuffer
				buffer.Destination = destination
			}
		} else {
			readBuffer := s.options.NewPacketBuffer()
			destination, err = s.source.ReadPacket(readBuffer)
			if err != nil {
				readBuffer.Release()
			} else {
				buffer = N.NewPacketBuffer()
				buffer.Buffer = readBuffer
				buffer.Destination = destination
			}
		}

		if err != nil {
			if E.IsTimeout(err) {
				if setter, ok := s.source.(interface{ SetReadDeadline(time.Time) error }); ok {
					setter.SetReadDeadline(time.Time{})
				}
				if s.state.CompareAndSwap(stateActive, stateIdle) {
					s.returnToPool()
				}
				return
			}
			if !notFirstTime {
				err = N.ReportHandshakeFailure(s.originSource, err)
			}
			s.closeWithError(err)
			return
		}

		err = s.writePacketWithCounters(buffer)
		if err != nil {
			if !notFirstTime {
				err = N.ReportHandshakeFailure(s.originSource, err)
			}
			s.closeWithError(err)
			return
		}
		notFirstTime = true
	}
}

func (s *reactorStream) writePacketWithCounters(packet *N.PacketBuffer) error {
	buffer := packet.Buffer
	destination := packet.Destination
	dataLen := buffer.Len()

	s.options.PostReturn(buffer)
	err := s.destination.WritePacket(buffer, destination)
	N.PutPacketBuffer(packet)
	if err != nil {
		buffer.Leak()
		return err
	}

	for _, counter := range s.readCounters {
		counter(int64(dataLen))
	}
	for _, counter := range s.writeCounters {
		counter(int64(dataLen))
	}
	return nil
}

func (s *reactorStream) returnToPool() {
	if s.state.Load() != stateIdle {
		return
	}

	switch notifier := s.notifier.(type) {
	case *N.ChannelNotifier:
		s.connection.reactor.channelDemux.Add(s, notifier.Channel)
		if s.state.Load() != stateIdle {
			s.connection.reactor.channelDemux.Remove(notifier.Channel)
		}
	case *N.FileDescriptorNotifier:
		if s.connection.reactor.fdDemux != nil {
			err := s.connection.reactor.fdDemux.Add(s, notifier.FD)
			if err != nil {
				s.closeWithError(err)
				return
			}
			if s.state.Load() != stateIdle {
				s.connection.reactor.fdDemux.Remove(notifier.FD)
			}
		}
	}
}

func (s *reactorStream) runLegacyCopy() {
	_, err := CopyPacket(s.destination, s.source)
	s.closeWithError(err)
}

func (s *reactorStream) closeWithError(err error) {
	s.connection.closeWithError(err)
}

func (c *reactorConnection) closeWithError(err error) {
	c.closeOnce.Do(func() {
		c.err = err
		c.cancel()

		if c.upload != nil {
			c.upload.state.Store(stateClosed)
		}
		if c.download != nil {
			c.download.state.Store(stateClosed)
		}

		c.removeFromDemultiplexers()

		if c.upload != nil {
			common.Close(c.upload.originSource)
		}
		if c.download != nil {
			common.Close(c.download.originSource)
		}

		if c.onClose != nil {
			c.onClose(c.err)
		}

		close(c.done)
	})
}

func (c *reactorConnection) removeFromDemultiplexers() {
	if c.upload != nil && c.upload.notifier != nil {
		switch notifier := c.upload.notifier.(type) {
		case *N.ChannelNotifier:
			c.reactor.channelDemux.Remove(notifier.Channel)
		case *N.FileDescriptorNotifier:
			if c.reactor.fdDemux != nil {
				c.reactor.fdDemux.Remove(notifier.FD)
			}
		}
	}
	if c.download != nil && c.download.notifier != nil {
		switch notifier := c.download.notifier.(type) {
		case *N.ChannelNotifier:
			c.reactor.channelDemux.Remove(notifier.Channel)
		case *N.FileDescriptorNotifier:
			if c.reactor.fdDemux != nil {
				c.reactor.fdDemux.Remove(notifier.FD)
			}
		}
	}
}
