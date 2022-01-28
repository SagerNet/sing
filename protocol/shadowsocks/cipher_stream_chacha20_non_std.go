//go:build !no_shadowsocks_stream && !(arm64 || ppc64le || s390x)

package shadowsocks

import (
	"crypto/cipher"

	"github.com/aead/chacha20"
	"github.com/aead/chacha20/chacha"
)

func init() {
	RegisterCipher("chacha20", func() Cipher {
		return &StreamCipher{
			KeyLength: chacha.KeySize,
			IVLength:  chacha.NonceSize,
			EncryptConstructor: func(key []byte, iv []byte) (cipher.Stream, error) {
				return chacha20.NewCipher(iv, key)
			},
			DecryptConstructor: func(key []byte, iv []byte) (cipher.Stream, error) {
				return chacha20.NewCipher(iv, key)
			},
		}
	})
	RegisterCipher("xchacha20", func() Cipher {
		return &StreamCipher{
			KeyLength: chacha.KeySize,
			IVLength:  chacha.XNonceSize,
			EncryptConstructor: func(key []byte, iv []byte) (cipher.Stream, error) {
				return chacha20.NewCipher(iv, key)
			},
			DecryptConstructor: func(key []byte, iv []byte) (cipher.Stream, error) {
				return chacha20.NewCipher(iv, key)
			},
		}
	})
}
