package bufio

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	N "github.com/sagernet/sing/common/network"
)

const (
	streamBatchReadTimeout = 250 * time.Millisecond
)

func CreateStreamPollable(reader io.Reader) (N.StreamPollable, bool) {
	if pollable, ok := reader.(N.StreamPollable); ok {
		return pollable, true
	}
	if creator, ok := reader.(N.StreamPollableCreator); ok {
		return creator.CreateStreamPollable()
	}
	if u, ok := reader.(N.ReaderWithUpstream); !ok || !u.ReaderReplaceable() {
		return nil, false
	}
	if u, ok := reader.(N.WithUpstreamReader); ok {
		return CreateStreamPollable(u.UpstreamReader().(io.Reader))
	}
	if u, ok := reader.(common.WithUpstream); ok {
		return CreateStreamPollable(u.Upstream().(io.Reader))
	}
	return nil, false
}

type StreamReactor struct {
	ctx          context.Context
	cancel       context.CancelFunc
	logger       logger.Logger
	fdPoller     *FDPoller
	fdPollerOnce sync.Once
	fdPollerErr  error
}

func NewStreamReactor(ctx context.Context, l logger.Logger) *StreamReactor {
	ctx, cancel := context.WithCancel(ctx)
	if l == nil {
		l = logger.NOP()
	}
	return &StreamReactor{
		ctx:    ctx,
		cancel: cancel,
		logger: l,
	}
}

func (r *StreamReactor) getFDPoller() (*FDPoller, error) {
	r.fdPollerOnce.Do(func() {
		r.fdPoller, r.fdPollerErr = NewFDPoller(r.ctx)
	})
	return r.fdPoller, r.fdPollerErr
}

func (r *StreamReactor) Close() error {
	r.cancel()
	if r.fdPoller != nil {
		return r.fdPoller.Close()
	}
	return nil
}

type streamConnection struct {
	ctx              context.Context
	cancel           context.CancelFunc
	reactor          *StreamReactor
	onClose          N.CloseHandlerFunc
	upload           *streamDirection
	download         *streamDirection
	stopReactorWatch func() bool

	closeOnce sync.Once
	done      chan struct{}
	err       error
}

type streamDirection struct {
	connection *streamConnection

	source       io.Reader
	destination  io.Writer
	originSource net.Conn

	pollable      N.StreamPollable
	options       N.ReadWaitOptions
	readWaiter    N.ReadWaiter
	readCounters  []N.CountFunc
	writeCounters []N.CountFunc

	isUpload bool
	state    atomic.Int32
}

// Copy initiates bidirectional TCP copy with reactor optimization
// It uses splice when available for zero-copy, otherwise falls back to reactor mode
func (r *StreamReactor) Copy(ctx context.Context, source net.Conn, destination net.Conn, onClose N.CloseHandlerFunc) {
	// Try splice first (zero-copy optimization)
	if r.trySplice(ctx, source, destination, onClose) {
		r.logger.Trace("stream copy: using splice for zero-copy")
		return
	}

	// Fall back to reactor mode
	r.logger.Trace("stream copy: using reactor mode")
	ctx, cancel := context.WithCancel(ctx)
	conn := &streamConnection{
		ctx:     ctx,
		cancel:  cancel,
		reactor: r,
		onClose: onClose,
		done:    make(chan struct{}),
	}
	conn.stopReactorWatch = common.ContextAfterFunc(r.ctx, func() {
		conn.closeWithError(r.ctx.Err())
	})

	conn.upload = r.prepareDirection(conn, source, destination, source, true)
	select {
	case <-conn.done:
		return
	default:
	}

	conn.download = r.prepareDirection(conn, destination, source, destination, false)
	select {
	case <-conn.done:
		return
	default:
	}

	r.registerDirection(conn.upload)
	r.registerDirection(conn.download)
}

func (r *StreamReactor) trySplice(ctx context.Context, source net.Conn, destination net.Conn, onClose N.CloseHandlerFunc) bool {
	if !N.SyscallAvailableForRead(source) || !N.SyscallAvailableForWrite(destination) {
		return false
	}

	// Both ends support syscall, use traditional copy with splice
	go func() {
		err := CopyConn(ctx, source, destination)
		if onClose != nil {
			onClose(err)
		}
	}()
	return true
}

func (r *StreamReactor) prepareDirection(conn *streamConnection, source io.Reader, destination io.Writer, originConn net.Conn, isUpload bool) *streamDirection {
	direction := &streamDirection{
		connection:   conn,
		source:       source,
		destination:  destination,
		originSource: originConn,
		isUpload:     isUpload,
	}

	// Unwrap counters and handle cached data
	for {
		source, direction.readCounters = N.UnwrapCountReader(source, direction.readCounters)
		destination, direction.writeCounters = N.UnwrapCountWriter(destination, direction.writeCounters)
		if cachedReader, isCached := source.(N.CachedReader); isCached {
			cachedBuffer := cachedReader.ReadCached()
			if cachedBuffer != nil {
				dataLen := cachedBuffer.Len()
				_, err := destination.Write(cachedBuffer.Bytes())
				cachedBuffer.Release()
				if err != nil {
					conn.closeWithError(err)
					return direction
				}
				for _, counter := range direction.readCounters {
					counter(int64(dataLen))
				}
				for _, counter := range direction.writeCounters {
					counter(int64(dataLen))
				}
				continue
			}
		}
		break
	}
	direction.source = source
	direction.destination = destination

	direction.options = N.NewReadWaitOptions(source, destination)

	direction.readWaiter, _ = CreateReadWaiter(source)
	if direction.readWaiter != nil {
		needCopy := direction.readWaiter.InitializeReadWaiter(direction.options)
		if needCopy {
			direction.readWaiter = nil
		}
	}

	// Try to get stream pollable for FD-based idle detection
	direction.pollable, _ = CreateStreamPollable(source)

	return direction
}

func (r *StreamReactor) registerDirection(direction *streamDirection) {
	// Check if there's buffered data that needs processing first
	if direction.pollable != nil && direction.pollable.Buffered() > 0 {
		r.logger.Trace("stream direction: has buffered data, starting active loop")
		go direction.runActiveLoop()
		return
	}

	// Try to register with FD poller
	if direction.pollable != nil {
		fdPoller, err := r.getFDPoller()
		if err == nil {
			err = fdPoller.Add(direction, direction.pollable.FD())
			if err == nil {
				r.logger.Trace("stream direction: registered with FD poller")
				return
			}
		}
	}

	// Fall back to legacy goroutine copy
	r.logger.Trace("stream direction: using legacy copy")
	go direction.runLegacyCopy()
}

func (d *streamDirection) runActiveLoop() {
	if d.source == nil {
		return
	}
	if !d.state.CompareAndSwap(stateIdle, stateActive) {
		return
	}

	notFirstTime := false

	for {
		if d.state.Load() == stateClosed {
			return
		}

		// Set batch read timeout
		if setter, ok := d.originSource.(interface{ SetReadDeadline(time.Time) error }); ok {
			setter.SetReadDeadline(time.Now().Add(streamBatchReadTimeout))
		}

		var (
			buffer *buf.Buffer
			err    error
		)

		if d.readWaiter != nil {
			buffer, err = d.readWaiter.WaitReadBuffer()
		} else {
			buffer = d.options.NewBuffer()
			err = NewExtendedReader(d.source).ReadBuffer(buffer)
			if err != nil {
				buffer.Release()
				buffer = nil
			}
		}

		if err != nil {
			if E.IsTimeout(err) {
				// Timeout: check buffer and return to pool
				if setter, ok := d.originSource.(interface{ SetReadDeadline(time.Time) error }); ok {
					setter.SetReadDeadline(time.Time{})
				}
				if d.state.CompareAndSwap(stateActive, stateIdle) {
					d.connection.reactor.logger.Trace("stream direction: timeout, returning to idle pool")
					d.returnToPool()
				}
				return
			}
			// EOF or error
			if !notFirstTime {
				err = N.ReportHandshakeFailure(d.originSource, err)
			}
			d.handleEOFOrError(err)
			return
		}

		err = d.writeBufferWithCounters(buffer)
		if err != nil {
			if !notFirstTime {
				err = N.ReportHandshakeFailure(d.originSource, err)
			}
			d.closeWithError(err)
			return
		}
		notFirstTime = true
	}
}

func (d *streamDirection) writeBufferWithCounters(buffer *buf.Buffer) error {
	dataLen := buffer.Len()
	d.options.PostReturn(buffer)
	err := NewExtendedWriter(d.destination).WriteBuffer(buffer)
	if err != nil {
		buffer.Leak()
		return err
	}

	for _, counter := range d.readCounters {
		counter(int64(dataLen))
	}
	for _, counter := range d.writeCounters {
		counter(int64(dataLen))
	}
	return nil
}

func (d *streamDirection) returnToPool() {
	if d.state.Load() != stateIdle {
		return
	}

	// Critical: check if there's buffered data before returning to idle
	if d.pollable != nil && d.pollable.Buffered() > 0 {
		go d.runActiveLoop()
		return
	}

	// Safe to wait for FD events
	if d.pollable != nil && d.connection.reactor.fdPoller != nil {
		err := d.connection.reactor.fdPoller.Add(d, d.pollable.FD())
		if err != nil {
			d.closeWithError(err)
			return
		}
		if d.state.Load() != stateIdle {
			d.connection.reactor.fdPoller.Remove(d.pollable.FD())
		}
	}
}

func (d *streamDirection) HandleFDEvent() {
	d.runActiveLoop()
}

func (d *streamDirection) runLegacyCopy() {
	_, err := CopyWithCounters(d.destination, d.source, d.originSource, d.readCounters, d.writeCounters, DefaultIncreaseBufferAfter, DefaultBatchSize)
	d.handleEOFOrError(err)
}

func (d *streamDirection) handleEOFOrError(err error) {
	if err == nil || err == io.EOF {
		// Graceful EOF: close write direction only (half-close)
		d.connection.reactor.logger.Trace("stream direction: graceful EOF, half-closing")
		d.state.Store(stateClosed)

		// Try half-close on destination
		if d.isUpload {
			if d.connection.download != nil {
				N.CloseWrite(d.connection.download.originSource)
			}
		} else {
			if d.connection.upload != nil {
				N.CloseWrite(d.connection.upload.originSource)
			}
		}

		d.connection.checkBothClosed()
		return
	}

	// Error: close entire connection
	d.connection.reactor.logger.Trace("stream direction: error occurred: ", err)
	d.closeWithError(err)
}

func (d *streamDirection) closeWithError(err error) {
	d.connection.closeWithError(err)
}

func (c *streamConnection) checkBothClosed() {
	uploadClosed := c.upload != nil && c.upload.state.Load() == stateClosed
	downloadClosed := c.download != nil && c.download.state.Load() == stateClosed

	if uploadClosed && downloadClosed {
		c.closeOnce.Do(func() {
			defer close(c.done)
			c.reactor.logger.Trace("stream connection: both directions closed gracefully")

			if c.stopReactorWatch != nil {
				c.stopReactorWatch()
			}

			c.cancel()
			c.removeFromPoller()

			common.Close(c.upload.originSource)
			common.Close(c.download.originSource)

			if c.onClose != nil {
				c.onClose(nil)
			}
		})
	}
}

func (c *streamConnection) closeWithError(err error) {
	c.closeOnce.Do(func() {
		defer close(c.done)
		c.reactor.logger.Trace("stream connection: closing with error: ", err)

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

		c.removeFromPoller()

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

func (c *streamConnection) removeFromPoller() {
	if c.reactor.fdPoller == nil {
		return
	}

	if c.upload != nil && c.upload.pollable != nil {
		c.reactor.fdPoller.Remove(c.upload.pollable.FD())
	}
	if c.download != nil && c.download.pollable != nil {
		c.reactor.fdPoller.Remove(c.download.pollable.FD())
	}
}
