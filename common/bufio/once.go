package bufio

import (
	"io"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	N "github.com/sagernet/sing/common/network"
)

func CopyOnce(dst io.Writer, src io.Reader) (n int64, err error) {
	extendedSrc, srcExtended := src.(N.ExtendedReader)
	extendedDst, dstExtended := dst.(N.ExtendedWriter)
	if !srcExtended {
		extendedSrc = &ExtendedReaderWrapper{src}
	}
	if !dstExtended {
		extendedDst = &ExtendedWriterWrapper{dst}
	}
	return CopyExtendedOnce(extendedDst, extendedSrc)
}

func CopyExtendedOnce(dst N.ExtendedWriter, src N.ExtendedReader) (n int64, err error) {
	var buffer *buf.Buffer
	if _, unsafe := common.Cast[N.ThreadUnsafeWriter](dst); unsafe {
		buffer = buf.New()
	} else {
		_buffer := buf.StackNew()
		defer common.KeepAlive(_buffer)
		buffer = common.Dup(_buffer)
	}
	err = src.ReadBuffer(buffer)
	if err != nil {
		buffer.Release()
		return
	}
	dataLen := buffer.Len()
	err = dst.WriteBuffer(buffer)
	if err != nil {
		buffer.Release()
		return
	}
	n += int64(dataLen)
	return
}

type ReaderFromWriter interface {
	io.ReaderFrom
	io.Writer
}

func ReadFrom0(readerFrom ReaderFromWriter, reader io.Reader) (n int64, err error) {
	n, err = CopyOnce(readerFrom, reader)
	if err != nil {
		return
	}
	var rn int64
	rn, err = readerFrom.ReadFrom(reader)
	if err != nil {
		return
	}
	n += rn
	return
}
