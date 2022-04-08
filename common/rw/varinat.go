package rw

import (
	"encoding/binary"
	"github.com/sagernet/sing/common"
	"io"
)

type InputStream interface {
	io.Reader
	io.ByteReader
}

type OutputStream interface {
	io.Writer
	io.ByteWriter
}

func WriteUVariant(writer io.Writer, value uint64) error {
	var b [8]byte
	return common.Error(writer.Write(b[:binary.PutUvarint(b[:], value)]))
}

func WriteVString(writer io.Writer, value string) error {
	err := WriteUVariant(writer, uint64(len(value)))
	if err != nil {
		return err
	}
	return WriteString(writer, value)
}

func ReadVString(reader InputStream) (string, error) {
	length, err := binary.ReadUvarint(reader)
	if err != nil {
		return "", err
	}
	value, err := ReadBytes(reader, int(length))
	if err != nil {
		return "", err
	}
	return string(value), nil
}
