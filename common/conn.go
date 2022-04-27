package common

import (
	"io"
	"net"
	"time"
)

type ReadOnlyException struct{}

func (e *ReadOnlyException) Error() string {
	return "read only connection"
}

type WriteOnlyException struct{}

func (e *WriteOnlyException) Error() string {
	return "write only connection"
}

type readWriteConn struct {
	io.Reader
	io.Writer
}

func (r *readWriteConn) Close() error {
	Close(r.Reader)
	return nil
}

func (r *readWriteConn) LocalAddr() net.Addr {
	return &DummyAddr{}
}

func (r *readWriteConn) RemoteAddr() net.Addr {
	return &DummyAddr{}
}

func (r *readWriteConn) SetDeadline(t time.Time) error {
	return nil
}

func (r *readWriteConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (r *readWriteConn) SetWriteDeadline(t time.Time) error {
	return nil
}

type readConn struct {
	readWriteConn
}

func (r *readConn) Write(b []byte) (n int, err error) {
	return 0, &ReadOnlyException{}
}

type writeConn struct {
	readWriteConn
	io.Writer
}

func (w *writeConn) Read(p []byte) (n int, err error) {
	return 0, &WriteOnlyException{}
}

func NewReadConn(reader io.Reader) net.Conn {
	c := &readConn{}
	c.Reader = reader
	return c
}

func NewWritConn(writer io.Writer) net.Conn {
	c := &writeConn{}
	c.Writer = writer
	return c
}

func NewReadWriteConn(reader io.Reader, writer io.Writer) net.Conn {
	c := &readConn{}
	c.Reader = reader
	c.Writer = writer
	return c
}
