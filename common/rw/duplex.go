package rw

import (
	"github.com/sagernet/sing/common"
)

type ReadCloser interface {
	CloseRead() error
}

type WriteCloser interface {
	CloseWrite() error
}

func CloseRead(reader any) error {
	r := reader
	for {
		if closer, ok := r.(ReadCloser); ok {
			return closer.CloseRead()
		}
		if u, ok := r.(common.ReaderWithUpstream); ok {
			r = u.UpstreamReader()
			continue
		}
		break
	}
	return common.Close(reader)
}

func CloseWrite(writer any) error {
	w := writer
	for {
		if closer, ok := w.(WriteCloser); ok {
			return closer.CloseWrite()
		}
		if u, ok := w.(common.WriterWithUpstream); ok {
			w = u.UpstreamWriter()
			continue
		}
		break
	}
	return common.Close(writer)
}
