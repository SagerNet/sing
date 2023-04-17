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

type readResult struct {
	buffer *buf.Buffer
	err    error
}

type Reader struct {
	N.ExtendedReader
	timeoutReader TimeoutReader
	deadline      time.Time
	pipeDeadline  pipeDeadline
	disablePipe   atomic.Bool
	inRead        atomic.Bool
	resultCh      chan *readResult
}

func NewReader(reader TimeoutReader) *Reader {
	r := &Reader{ExtendedReader: bufio.NewExtendedReader(reader), timeoutReader: reader, pipeDeadline: makePipeDeadline(), resultCh: make(chan *readResult, 1)}
	r.resultCh <- nil
	return r
}

func (r *Reader) Read(p []byte) (n int, err error) {
	select {
	case result := <-r.resultCh:
		if result != nil {
			n = copy(p, result.buffer.Bytes())
			result.buffer.Advance(n)
			if result.buffer.IsEmpty() {
				result.buffer.Release()
				err = result.err
				r.resultCh <- nil // finish cache read
			} else {
				r.resultCh <- result // push back for next call
			}
			return
		} else {
			r.resultCh <- nil
			break
		}
	case <-r.pipeDeadline.wait():
		return 0, os.ErrDeadlineExceeded
	}

	if r.disablePipe.Load() {
		return r.ExtendedReader.Read(p)
	} else if r.deadline.IsZero() {
		r.inRead.Store(true)
		defer r.inRead.Store(false)
		n, err = r.ExtendedReader.Read(p)
		return
	}

	<-r.resultCh
	go r.pipeRead(len(p))

	return r.Read(p)
}

func (r *Reader) pipeRead(size int) {
	buffer := buf.NewSize(size)
	_, err := buffer.ReadOnceFrom(r.ExtendedReader)
	r.resultCh <- &readResult{
		buffer: buffer,
		err:    err,
	}
}

func (r *Reader) ReadBuffer(buffer *buf.Buffer) error {
	select {
	case result := <-r.resultCh:
		if result != nil {
			buffer.Resize(result.buffer.Start(), 0)
			n := copy(buffer.FreeBytes(), result.buffer.Bytes())
			result.buffer.Advance(n)
			if result.buffer.IsEmpty() {
				result.buffer.Release()
				r.resultCh <- nil // finish cache read
				return result.err
			} else {
				r.resultCh <- result // push back for next call
				return nil
			}
		} else {
			r.resultCh <- nil
			break
		}
	case <-r.pipeDeadline.wait():
		return os.ErrDeadlineExceeded
	}

	if r.disablePipe.Load() {
		return r.ExtendedReader.ReadBuffer(buffer)
	} else if r.deadline.IsZero() {
		r.inRead.Store(true)
		defer r.inRead.Store(false)
		return r.ExtendedReader.ReadBuffer(buffer)
	}

	<-r.resultCh
	go r.pipeReadBuffer(buffer.Cap(), buffer.Start())

	return r.ReadBuffer(buffer)
}

func (r *Reader) pipeReadBuffer(cap, start int) {
	cacheBuffer := buf.NewSize(cap)
	cacheBuffer.Resize(start, 0)
	err := r.ExtendedReader.ReadBuffer(cacheBuffer)
	r.resultCh <- &readResult{
		buffer: cacheBuffer,
		err:    err,
	}
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

func (r *Reader) UpstreamReader() any {
	return r.ExtendedReader
}
