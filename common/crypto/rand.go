package crypto

import (
	"crypto/rand"

	"sing/common"
)

func RandomBytes(size int) []byte {
	b := make([]byte, size)
	common.Must1(rand.Read(b))
	return b
}
