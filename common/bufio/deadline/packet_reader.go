package deadline

import (
	"net"
	"os"
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

type readPacketResult struct {
	buffer *buf.Buffer
	addr   M.Socksaddr
	err    error
}

type PacketReader struct {
	TimeoutPacketReader
	deadline     time.Time
	pipeDeadline pipeDeadline
	disablePipe  atomic.Bool
	inRead       atomic.Bool
	resultCh     chan *readPacketResult
}

func NewPacketReader(reader TimeoutPacketReader) *PacketReader {
	r := &PacketReader{TimeoutPacketReader: reader, pipeDeadline: makePipeDeadline(), resultCh: make(chan *readPacketResult, 1)}
	r.resultCh <- nil
	return r
}

func (r *PacketReader) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	select {
	case result := <-r.resultCh:
		if result != nil {
			n = copy(p, result.buffer.Bytes())
			addr = result.addr.UDPAddr()
			err = result.err
			result.buffer.Release()
			r.resultCh <- nil // finish cache read
		} else {
			r.resultCh <- nil
			break
		}
	case <-r.pipeDeadline.wait():
		return 0, nil, os.ErrDeadlineExceeded
	}

	if r.disablePipe.Load() {
		return r.TimeoutPacketReader.ReadFrom(p)
	} else if r.deadline.IsZero() {
		r.inRead.Store(true)
		defer r.inRead.Store(false)
		n, addr, err = r.TimeoutPacketReader.ReadFrom(p)
		return
	}

	<-r.resultCh
	go r.pipeReadFrom(len(p))

	return r.ReadFrom(p)
}

func (r *PacketReader) pipeReadFrom(size int) {
	cacheBuffer := buf.NewSize(size)
	n, addr, err := r.TimeoutPacketReader.ReadFrom(cacheBuffer.FreeBytes())
	cacheBuffer.Truncate(n)
	r.resultCh <- &readPacketResult{
		buffer: cacheBuffer,
		addr:   M.SocksaddrFromNet(addr),
		err:    err,
	}
}

func (r *PacketReader) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	select {
	case result := <-r.resultCh:
		if result != nil {
			destination = result.addr
			err = result.err
			buffer.Resize(result.buffer.Start(), 0)
			buffer.Write(result.buffer.Bytes())
			result.buffer.Release()
			r.resultCh <- nil // finish cache read
			return
		} else {
			r.resultCh <- nil
			break
		}
	case <-r.pipeDeadline.wait():
		return M.Socksaddr{}, os.ErrDeadlineExceeded
	}

	if r.disablePipe.Load() {
		return r.TimeoutPacketReader.ReadPacket(buffer)
	} else if r.deadline.IsZero() {
		r.inRead.Store(true)
		defer r.inRead.Store(false)
		destination, err = r.TimeoutPacketReader.ReadPacket(buffer)
		return
	}

	<-r.resultCh
	go r.pipeReadPacket(buffer.Cap(), buffer.Start())

	return r.ReadPacket(buffer)
}

func (r *PacketReader) pipeReadPacket(cap, start int) {
	cacheBuffer := buf.NewSize(cap)
	cacheBuffer.Resize(start, 0)
	destination, err := r.TimeoutPacketReader.ReadPacket(cacheBuffer)
	r.resultCh <- &readPacketResult{
		buffer: cacheBuffer,
		addr:   destination,
		err:    err,
	}
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
	select {
	case result := <-r.resultCh:
		r.resultCh <- result
		if result != nil {
			return false // cache reading
		} else {
			break
		}
	default:
		return false // pipe reading
	}
	return r.disablePipe.Load() || r.deadline.IsZero()
}

func (r *PacketReader) UpstreamReader() any {
	return r.TimeoutPacketReader
}
