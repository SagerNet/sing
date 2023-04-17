package deadline

import (
	"io"
	"os"
	"time"

	"github.com/sagernet/sing/common/atomic"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	N "github.com/sagernet/sing/common/network"
)

type TimeoutReader interface {
	io.Reader
	SetReadDeadline(t time.Time) error
}

type Reader struct {
	N.ExtendedReader
	timeoutReader TimeoutReader
	deadline      time.Time
	pipeDeadline  pipeDeadline
	disablePipe   atomic.Bool
	inRead        atomic.Bool
	result        chan *readResult
	done          chan struct{}
}

type readResult struct {
	buffer *buf.Buffer
	err    error
}

func NewReader(reader TimeoutReader) *Reader {
	return &Reader{
		ExtendedReader: bufio.NewExtendedReader(reader),
		timeoutReader:  reader,
		pipeDeadline:   makePipeDeadline(),
		result:         make(chan *readResult, 1),
		done:           makeFilledChan(),
	}
}

func (r *Reader) Read(p []byte) (n int, err error) {
	select {
	case result := <-r.result:
		return r.pipeReturn(result, p)
	default:
	}
	if r.disablePipe.Load() {
		return r.ExtendedReader.Read(p)
	} else if r.deadline.IsZero() {
		r.inRead.Store(true)
		defer r.inRead.Store(false)
		n, err = r.ExtendedReader.Read(p)
		return
	}
	select {
	case <-r.done:
		go r.pipeRead(len(p))
	default:
	}
	select {
	case result := <-r.result:
		return r.pipeReturn(result, p)
	case <-r.pipeDeadline.wait():
		return 0, os.ErrDeadlineExceeded
	}
}

func (r *Reader) pipeReturn(result *readResult, p []byte) (n int, err error) {
	n = copy(p, result.buffer.Bytes())
	result.buffer.Advance(n)
	if result.buffer.IsEmpty() {
		result.buffer.Release()
		err = result.err
	} else {
		r.result <- result
	}
	return
}

func (r *Reader) pipeRead(pLen int) {
	buffer := buf.NewSize(pLen)
	_, err := buffer.ReadOnceFrom(r.ExtendedReader)
	r.result <- &readResult{
		buffer: buffer,
		err:    err,
	}
	r.done <- struct{}{}
}

func (r *Reader) ReadBuffer(buffer *buf.Buffer) error {
	select {
	case result := <-r.result:
		return r.pipeReturnBuffer(result, buffer)
	default:
	}
	if r.disablePipe.Load() {
		return r.ExtendedReader.ReadBuffer(buffer)
	} else if r.deadline.IsZero() {
		r.inRead.Store(true)
		defer r.inRead.Store(false)
		return r.ExtendedReader.ReadBuffer(buffer)
	}
	select {
	case <-r.done:
		go r.pipeReadBuffer(buffer.Cap(), buffer.Start())
	default:
	}
	select {
	case result := <-r.result:
		return r.pipeReturnBuffer(result, buffer)
	case <-r.pipeDeadline.wait():
		return os.ErrDeadlineExceeded
	}
}

func (r *Reader) pipeReturnBuffer(result *readResult, buffer *buf.Buffer) error {
	buffer.Resize(result.buffer.Start(), 0)
	n := copy(buffer.FreeBytes(), result.buffer.Bytes())
	buffer.Truncate(n)
	result.buffer.Advance(n)
	if !result.buffer.IsEmpty() {
		r.result <- result
		return result.err
	} else {
		result.buffer.Release()
		return nil
	}
}

func (r *Reader) pipeReadBuffer(bufLen int, bufStart int) {
	cacheBuffer := buf.NewSize(bufLen)
	cacheBuffer.Advance(bufStart)
	err := r.ExtendedReader.ReadBuffer(cacheBuffer)
	r.result <- &readResult{
		buffer: cacheBuffer,
		err:    err,
	}
	r.done <- struct{}{}
}

func (r *Reader) SetReadDeadline(t time.Time) error {
	if r.disablePipe.Load() {
		return r.timeoutReader.SetReadDeadline(t)
	} else if r.inRead.Load() {
		r.disablePipe.Store(true)
		return r.timeoutReader.SetReadDeadline(t)
	}
	r.deadline = t
	r.pipeDeadline.set(t)
	return nil
}

func (r *Reader) ReaderReplaceable() bool {
	select {
	case <-r.done:
		r.done <- struct{}{}
	default:
		return false
	}
	select {
	case result := <-r.result:
		r.result <- result
		return false
	default:
	}
	return r.disablePipe.Load() || r.deadline.IsZero()
}

func (r *Reader) UpstreamReader() any {
	return r.ExtendedReader
}
