package network

import "io"

type ReadCloser interface {
	CloseRead() error
}

type WriteCloser interface {
	CloseWrite() error
}

func CloseRead(reader io.Reader) error {
	reader, _ = UnwrapCountReader(reader, nil)
	readCloser, isReadCloser := UnwrapReader(reader).(ReadCloser)
	if isReadCloser {
		return readCloser.CloseRead()
	}
	return nil
}

func CloseWrite(writer io.Writer) error {
	writer, _ = UnwrapCountWriter(writer, nil)
	writeCloser, isWriteCloser := UnwrapWriter(writer).(WriteCloser)
	if isWriteCloser {
		return writeCloser.CloseWrite()
	}
	return nil
}
