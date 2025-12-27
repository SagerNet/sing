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
	ctx           context.Context
	cancel        context.CancelFunc
	channelPoller *ChannelPoller
	fdPoller      *FDPoller
	fdPollerOnce  sync.Once
	fdPollerErr   error
}

func NewPacketReactor(ctx context.Context) *PacketReactor {
	ctx, cancel := context.WithCancel(ctx)
	return &PacketReactor{
		ctx:           ctx,
		cancel:        cancel,
		channelPoller: NewChannelPoller(ctx),
	}
}

func (r *PacketReactor) getFDPoller() (*FDPoller, error) {
	r.fdPollerOnce.Do(func() {
		r.fdPoller, r.fdPollerErr = NewFDPoller(r.ctx)
	})
	return r.fdPoller, r.fdPollerErr
}

func (r *PacketReactor) Close() error {
	r.cancel()
	var errs []error
	if r.channelPoller != nil {
		errs = append(errs, r.channelPoller.Close())
	}
	if r.fdPoller != nil {
		errs = append(errs, r.fdPoller.Close())
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

	pollable      N.PacketPollable
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

	if pollable, ok := source.(N.PacketPollable); ok {
		stream.pollable = pollable
	} else if creator, ok := source.(N.PacketPollableCreator); ok {
		stream.pollable, _ = creator.CreatePacketPollable()
	}

	return stream
}

func (r *PacketReactor) registerStream(stream *reactorStream) {
	if stream.pollable == nil {
		go stream.runLegacyCopy()
		return
	}

	switch stream.pollable.PollMode() {
	case N.PacketPollModeChannel:
		r.channelPoller.Add(stream, stream.pollable.PacketChannel())
	case N.PacketPollModeFD:
		fdPoller, err := r.getFDPoller()
		if err != nil {
			go stream.runLegacyCopy()
			return
		}
		err = fdPoller.Add(stream, stream.pollable.FD())
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

	if s.pollable == nil {
		return
	}

	switch s.pollable.PollMode() {
	case N.PacketPollModeChannel:
		channel := s.pollable.PacketChannel()
		s.connection.reactor.channelPoller.Add(s, channel)
		if s.state.Load() != stateIdle {
			s.connection.reactor.channelPoller.Remove(channel)
		}
	case N.PacketPollModeFD:
		if s.connection.reactor.fdPoller == nil {
			return
		}
		fd := s.pollable.FD()
		err := s.connection.reactor.fdPoller.Add(s, fd)
		if err != nil {
			s.closeWithError(err)
			return
		}
		if s.state.Load() != stateIdle {
			s.connection.reactor.fdPoller.Remove(fd)
		}
	}
}

func (s *reactorStream) HandleFDEvent() {
	s.runActiveLoop(nil)
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

		c.removeFromPollers()

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

func (c *reactorConnection) removeFromPollers() {
	c.removeStreamFromPoller(c.upload)
	c.removeStreamFromPoller(c.download)
}

func (c *reactorConnection) removeStreamFromPoller(stream *reactorStream) {
	if stream == nil || stream.pollable == nil {
		return
	}
	switch stream.pollable.PollMode() {
	case N.PacketPollModeChannel:
		c.reactor.channelPoller.Remove(stream.pollable.PacketChannel())
	case N.PacketPollModeFD:
		if c.reactor.fdPoller != nil {
			c.reactor.fdPoller.Remove(stream.pollable.FD())
		}
	}
}
