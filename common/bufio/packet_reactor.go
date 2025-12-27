package bufio

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
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

func CreatePacketPushable(reader N.PacketReader) (N.PacketPushable, bool) {
	if pushable, ok := reader.(N.PacketPushable); ok {
		return pushable, true
	}
	if u, ok := reader.(N.ReaderWithUpstream); !ok || !u.ReaderReplaceable() {
		return nil, false
	}
	if u, ok := reader.(N.WithUpstreamReader); ok {
		if upstream, ok := u.UpstreamReader().(N.PacketReader); ok {
			return CreatePacketPushable(upstream)
		}
	}
	if u, ok := reader.(common.WithUpstream); ok {
		if upstream, ok := u.Upstream().(N.PacketReader); ok {
			return CreatePacketPushable(upstream)
		}
	}
	return nil, false
}

func CreatePacketPollable(reader N.PacketReader) (N.PacketPollable, bool) {
	if pollable, ok := reader.(N.PacketPollable); ok {
		return pollable, true
	}
	if creator, ok := reader.(N.PacketPollableCreator); ok {
		return creator.CreatePacketPollable()
	}
	if u, ok := reader.(N.ReaderWithUpstream); !ok || !u.ReaderReplaceable() {
		return nil, false
	}
	if u, ok := reader.(N.WithUpstreamReader); ok {
		if upstream, ok := u.UpstreamReader().(N.PacketReader); ok {
			return CreatePacketPollable(upstream)
		}
	}
	if u, ok := reader.(common.WithUpstream); ok {
		if upstream, ok := u.Upstream().(N.PacketReader); ok {
			return CreatePacketPollable(upstream)
		}
	}
	return nil, false
}

type PacketReactor struct {
	ctx          context.Context
	cancel       context.CancelFunc
	logger       logger.Logger
	fdPoller     *FDPoller
	fdPollerOnce sync.Once
	fdPollerErr  error
}

func NewPacketReactor(ctx context.Context, l logger.Logger) *PacketReactor {
	ctx, cancel := context.WithCancel(ctx)
	if l == nil {
		l = logger.NOP()
	}
	return &PacketReactor{
		ctx:    ctx,
		cancel: cancel,
		logger: l,
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
	if r.fdPoller != nil {
		return r.fdPoller.Close()
	}
	return nil
}

type reactorConnection struct {
	ctx              context.Context
	cancel           context.CancelFunc
	reactor          *PacketReactor
	onClose          N.CloseHandlerFunc
	upload           *reactorStream
	download         *reactorStream
	stopReactorWatch func() bool

	closeOnce sync.Once
	done      chan struct{}
	err       error
}

type reactorStream struct {
	connection *reactorConnection

	source       N.PacketReader
	destination  N.PacketWriter
	originSource N.PacketReader

	pushable      N.PacketPushable
	pollable      N.PacketPollable
	options       N.ReadWaitOptions
	readWaiter    N.PacketReadWaiter
	readCounters  []N.CountFunc
	writeCounters []N.CountFunc

	state atomic.Int32
}

func (r *PacketReactor) Copy(ctx context.Context, source N.PacketConn, destination N.PacketConn, onClose N.CloseHandlerFunc) {
	r.logger.Trace("packet copy: starting")
	ctx, cancel := context.WithCancel(ctx)
	conn := &reactorConnection{
		ctx:     ctx,
		cancel:  cancel,
		reactor: r,
		onClose: onClose,
		done:    make(chan struct{}),
	}
	conn.stopReactorWatch = common.ContextAfterFunc(r.ctx, func() {
		conn.closeWithError(r.ctx.Err())
	})

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
				buffer := packet.Buffer
				dataLen := buffer.Len()
				err := destination.WritePacket(buffer, packet.Destination)
				N.PutPacketBuffer(packet)
				if err != nil {
					buffer.Leak()
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
		needCopy := stream.readWaiter.InitializeReadWaiter(stream.options)
		if needCopy {
			stream.readWaiter = nil
		}
	}

	stream.pushable, _ = CreatePacketPushable(source)
	if stream.pushable == nil {
		stream.pollable, _ = CreatePacketPollable(source)
	}

	return stream
}

func (r *PacketReactor) registerStream(stream *reactorStream) {
	if stream.pushable != nil {
		r.logger.Trace("packet stream: using pushable mode")
		stream.pushable.SetOnDataReady(func() {
			go stream.runActiveLoop(nil)
		})
		if stream.pushable.HasPendingData() {
			go stream.runActiveLoop(nil)
		}
		return
	}

	if stream.pollable == nil {
		r.logger.Trace("packet stream: using legacy copy")
		go stream.runLegacyCopy()
		return
	}

	fdPoller, err := r.getFDPoller()
	if err != nil {
		r.logger.Trace("packet stream: FD poller unavailable, using legacy copy")
		go stream.runLegacyCopy()
		return
	}
	err = fdPoller.Add(stream, stream.pollable.FD())
	if err != nil {
		r.logger.Trace("packet stream: failed to add to FD poller, using legacy copy")
		go stream.runLegacyCopy()
	} else {
		r.logger.Trace("packet stream: registered with FD poller")
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
				if !s.state.CompareAndSwap(stateActive, stateIdle) {
					return
				}
				s.connection.reactor.logger.Trace("packet stream: timeout, returning to idle pool")
				if s.pushable != nil {
					if s.pushable.HasPendingData() {
						if s.state.CompareAndSwap(stateIdle, stateActive) {
							continue
						}
					}
					return
				}
				s.returnToPool()
				return
			}
			if !notFirstTime {
				err = N.ReportHandshakeFailure(s.originSource, err)
			}
			s.connection.reactor.logger.Trace("packet stream: error occurred: ", err)
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

	if s.pollable == nil || s.connection.reactor.fdPoller == nil {
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

func (s *reactorStream) HandleFDEvent() {
	s.runActiveLoop(nil)
}

func (s *reactorStream) runLegacyCopy() {
	_, err := CopyPacketWithCounters(s.destination, s.source, s.originSource, s.readCounters, s.writeCounters)
	s.closeWithError(err)
}

func (s *reactorStream) closeWithError(err error) {
	s.connection.closeWithError(err)
}

func (c *reactorConnection) closeWithError(err error) {
	c.closeOnce.Do(func() {
		defer close(c.done)
		c.reactor.logger.Trace("packet connection: closing with error: ", err)

		if c.stopReactorWatch != nil {
			c.stopReactorWatch()
		}

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
	})
}

func (c *reactorConnection) removeFromPollers() {
	c.removeStreamFromPoller(c.upload)
	c.removeStreamFromPoller(c.download)
}

func (c *reactorConnection) removeStreamFromPoller(stream *reactorStream) {
	if stream == nil || stream.pollable == nil || c.reactor.fdPoller == nil {
		return
	}
	c.reactor.fdPoller.Remove(stream.pollable.FD())
}
