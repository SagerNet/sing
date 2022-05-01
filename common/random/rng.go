package random

import (
	"crypto/rand"
	"encoding/binary"
	"io"
	"sync"

	"github.com/sagernet/sing/common"
	"lukechampine.com/blake3"
)

var System = &Source{rand.Reader}

var Default = Blake3KeyedHash()

func Blake3KeyedHash() *Source {
	key := make([]byte, 32)
	common.Must1(io.ReadFull(System, key))
	h := blake3.New(1024, key)
	return &Source{&SyncReader{Reader: h.XOF()}}
}

type SyncReader struct {
	io.Reader
	sync.Mutex
}

func (r *SyncReader) Read(p []byte) (n int, err error) {
	r.Lock()
	defer r.Unlock()
	return r.Reader.Read(p)
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
