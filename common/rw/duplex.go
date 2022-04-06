package rw

import "io"

type ReadCloser interface {
	CloseRead() error
}

type WriteCloser interface {
	CloseWrite() error
}

func CloseRead(conn io.Closer) error {
	if closer, ok := conn.(ReadCloser); ok {
		return closer.CloseRead()
	}
	return conn.Close()
}

func CloseWrite(conn io.Closer) error {
	if closer, ok := conn.(WriteCloser); ok {
		return closer.CloseWrite()
	}
	return conn.Close()
}
