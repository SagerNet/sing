package deadline

import (
	"net"
	"os"
	"sync"
	"time"

	"github.com/sagernet/sing/common/atomic"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type TimeoutPacketReader interface {
	N.NetPacketReader
	SetReadDeadline(t time.Time) error
}

type PacketReader struct {
	TimeoutPacketReader
	deadline     time.Time
	pipeDeadline pipeDeadline
	disablePipe  atomic.Bool
	inRead       atomic.Bool
	cacheAccess  sync.RWMutex
	cached       bool
	cachedBuffer *buf.Buffer
	cachedAddr   M.Socksaddr
	cachedErr    error
}

func NewPacketReader(reader TimeoutPacketReader) *PacketReader {
	return &PacketReader{TimeoutPacketReader: reader, pipeDeadline: makePipeDeadline()}
}

func (r *PacketReader) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	if r.disablePipe.Load() {
		return r.TimeoutPacketReader.ReadFrom(p)
	} else if r.deadline.IsZero() {
		r.inRead.Store(true)
		defer r.inRead.Store(false)
		n, addr, err = r.TimeoutPacketReader.ReadFrom(p)
		return
	}
	r.cacheAccess.Lock()
	if r.cached {
		n = copy(p, r.cachedBuffer.Bytes())
		addr = r.cachedAddr.UDPAddr()
		err = r.cachedErr
		r.cachedBuffer.Release()
		r.cached = false
		r.cacheAccess.Unlock()
		return
	}
	r.cacheAccess.Unlock()
	done := make(chan struct{})
	var access sync.Mutex
	var cancel bool
	go func() {
		n, addr, err = r.pipeReadFrom(p, &access, &cancel, done)
	}()
	select {
	case <-done:
		return
	case <-r.pipeDeadline.wait():
	}
	access.Lock()
	defer access.Unlock()
	select {
	case <-done:
		return
	default:
	}
	cancel = true
	return 0, nil, os.ErrDeadlineExceeded
}

func (r *PacketReader) pipeReadFrom(p []byte, access *sync.Mutex, cancel *bool, done chan struct{}) (n int, addr net.Addr, err error) {
	r.cacheAccess.Lock()
	defer r.cacheAccess.Unlock()
	cacheBuffer := buf.NewSize(len(p))
	n, addr, err = r.TimeoutPacketReader.ReadFrom(cacheBuffer.Bytes())
	access.Lock()
	defer access.Unlock()
	if *cancel {
		r.cached = true
		r.cachedBuffer = cacheBuffer
		r.cachedAddr = M.SocksaddrFromNet(addr)
		r.cachedErr = err
	} else {
		copy(p, cacheBuffer.Bytes())
		cacheBuffer.Release()
	}
	close(done)
	return
}

func (r *PacketReader) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	if r.disablePipe.Load() {
		return r.TimeoutPacketReader.ReadPacket(buffer)
	} else if r.deadline.IsZero() {
		r.inRead.Store(true)
		defer r.inRead.Store(false)
		destination, err = r.TimeoutPacketReader.ReadPacket(buffer)
		return
	}
	r.cacheAccess.Lock()
	if r.cached {
		destination = r.cachedAddr
		err = r.cachedErr
		buffer.Resize(r.cachedBuffer.Start(), 0)
		buffer.Write(r.cachedBuffer.Bytes())
		r.cachedBuffer.Release()
		r.cached = false
		r.cacheAccess.Unlock()
		return
	}
	r.cacheAccess.Unlock()
	done := make(chan struct{})
	var access sync.Mutex
	var cancel bool
	go func() {
		destination, err = r.pipeReadPacket(buffer, &access, &cancel, done)
	}()
	select {
	case <-done:
		return
	case <-r.pipeDeadline.wait():
	}
	access.Lock()
	defer access.Unlock()
	select {
	case <-done:
		return
	default:
	}
	cancel = true
	return M.Socksaddr{}, os.ErrDeadlineExceeded
}

func (r *PacketReader) pipeReadPacket(buffer *buf.Buffer, access *sync.Mutex, cancel *bool, done chan struct{}) (destination M.Socksaddr, err error) {
	r.cacheAccess.Lock()
	defer r.cacheAccess.Unlock()
	cacheBuffer := buf.NewSize(buffer.Cap())
	cacheBuffer.Resize(buffer.Start(), 0)
	destination, err = r.TimeoutPacketReader.ReadPacket(cacheBuffer)
	access.Lock()
	defer access.Unlock()
	if *cancel {
		r.cached = true
		r.cachedBuffer = cacheBuffer
		r.cachedAddr = destination
		r.cachedErr = err
	} else {
		buffer.Resize(cacheBuffer.Start(), 0)
		buffer.ReadOnceFrom(cacheBuffer)
		cacheBuffer.Release()
	}
	close(done)
	return
}

func (r *PacketReader) SetReadDeadline(t time.Time) error {
	if r.disablePipe.Load() {
		return r.TimeoutPacketReader.SetReadDeadline(t)
	} else if r.inRead.Load() {
		r.disablePipe.Store(true)
		return r.TimeoutPacketReader.SetReadDeadline(t)
	}
	r.deadline = t
	r.pipeDeadline.set(t)
	return nil
}

func (r *PacketReader) ReaderReplaceable() bool {
	return r.disablePipe.Load() || r.deadline.IsZero()
}

func (r *PacketReader) UpstreamReader() any {
	return r.TimeoutPacketReader
}
