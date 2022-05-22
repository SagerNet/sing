package rw

import (
	"io"
	"runtime"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
)

func WriteVC(conn io.Writer, data ...[]byte) (int, error) {
	if fd, err := common.TryFileDescriptor(conn); err == nil {
		return WriteV(fd, data...)
	}
	var bufLen int
	for _, dataItem := range data {
		bufLen += len(dataItem)
	}
	_buffer := buf.Make(bufLen)
	defer runtime.KeepAlive(_buffer)
	buffer := common.Dup(_buffer)
	var bufN int
	for _, dataItem := range data {
		bufN += copy(buffer[bufN:], dataItem)
	}
	return conn.Write(buffer)
}
