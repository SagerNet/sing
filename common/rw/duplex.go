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
	if c, ok := common.Cast[ReadCloser](reader); ok {
		return c.CloseRead()
	}
	return common.Close(reader)
}

func CloseWrite(writer any) error {
	if c, ok := common.Cast[WriteCloser](writer); ok {
		return c.CloseWrite()
	}
	return common.Close(writer)
}
