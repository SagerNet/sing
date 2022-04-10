package random

import (
	"crypto/rand"
	"io"

	"github.com/sagernet/sing/common"
	"lukechampine.com/blake3"
)

var System = rand.Reader

func Blake3KeyedHash() io.Reader {
	key := make([]byte, 32)
	common.Must1(io.ReadFull(System, key))
	h := blake3.New(1024, key)
	return h.XOF()
}
