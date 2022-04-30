package random

import (
	"crypto/rand"
	"encoding/binary"
	"io"

	"github.com/sagernet/sing/common"
	"lukechampine.com/blake3"
)

var System = rand.Reader

func Blake3KeyedHash() Source {
	key := make([]byte, 32)
	common.Must1(io.ReadFull(System, key))
	h := blake3.New(1024, key)
	return Source{h.XOF()}
}

const (
	rngMax  = 1 << 63
	rngMask = rngMax - 1
)

type Source struct {
	io.Reader
}

func (s Source) Int63() int64 {
	return s.Int64() & rngMask
}

func (s Source) Int64() int64 {
	var num int64
	common.Must(binary.Read(s, binary.BigEndian, &num))
	return num
}

func (s Source) Uint64() uint64 {
	var num uint64
	common.Must(binary.Read(s, binary.BigEndian, &num))
	return num
}

func (s Source) Seed(int64) {
}
