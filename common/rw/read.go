package rw

import (
	"io"
	"runtime"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
)

func Skip(reader io.Reader) error {
	return SkipN(reader, 1)
}

func SkipN(reader io.Reader, size int) error {
	return common.Error(ReadBytes(reader, size))
}

func ReadByte(reader io.Reader) (byte, error) {
	if br, isBr := reader.(io.ByteReader); isBr {
		return br.ReadByte()
	}
	var b [1]byte
	if err := common.Error(io.ReadFull(reader, b[:])); err != nil {
		return 0, err
	}
	return b[0], nil
}

func ReadBytes(reader io.Reader, size int) ([]byte, error) {
	b := make([]byte, size)
	if err := common.Error(io.ReadFull(reader, b[:])); err != nil {
		return nil, err
	}
	return b, nil
}

func ReadString(reader io.Reader, size int) (string, error) {
	b, err := ReadBytes(reader, size)
	if err != nil {
		return "", err
	}
	return string(b), nil
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

func CopyOnce(dest io.Writer, src io.Reader) (n int64, err error) {
	_buffer := buf.StackNew()
	defer runtime.KeepAlive(_buffer)
	buffer := common.Dup(_buffer)
	n, err = buffer.ReadFrom(src)
	if err != nil {
		return
	}
	_, err = dest.Write(buffer.Bytes())
	return
}
