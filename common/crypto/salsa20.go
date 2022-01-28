package crypto

import (
	"crypto/cipher"
	"encoding/binary"
	"errors"

	"golang.org/x/crypto/salsa20/salsa"
	"sing/common"
)

type Salsa20Cipher struct {
	nonce   []byte
	key     [32]byte
	counter uint64
}

func (s *Salsa20Cipher) XORKeyStream(dst, src []byte) {
	if len(dst) < len(src) {
		common.Must(errors.New("dst is smaller than src"))
	}
	padLen := int(s.counter % 64)
	buf := make([]byte, len(src)+padLen)

	var subNonce [16]byte
	copy(subNonce[:], s.nonce)
	binary.LittleEndian.PutUint64(subNonce[8:], s.counter/64)

	// It's difficult to avoid data copy here. src or dst maybe slice from
	// Conn.Read/Write, which can't have padding.
	copy(buf[padLen:], src)
	salsa.XORKeyStream(buf, buf, &subNonce, &s.key)
	copy(dst, buf[padLen:])

	s.counter += uint64(len(src))
}

func NewSalsa20(key []byte, nonce []byte) (cipher.Stream, error) {
	var fixedSizedKey [32]byte
	if len(key) != 32 {
		return nil, errors.New("key size must be 32")
	}
	copy(fixedSizedKey[:], key)
	return &Salsa20Cipher{
		key:   fixedSizedKey,
		nonce: nonce,
	}, nil
}
