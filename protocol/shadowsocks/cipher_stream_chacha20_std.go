//go:build !no_shadowsocks_stream && (arm64 || ppc64le || s390x)

package shadowsocks

import (
	"crypto/cipher"

	"golang.org/x/crypto/chacha20"
)

func init() {
	RegisterCipher("chacha20", func() Cipher {
		return &StreamCipher{
			KeyLength: chacha20.KeySize,
			IVLength:  chacha20.NonceSize,
			EncryptConstructor: func(key []byte, iv []byte) (cipher.Stream, error) {
				return chacha20.NewUnauthenticatedCipher(key, iv)
			},
			DecryptConstructor: func(key []byte, iv []byte) (cipher.Stream, error) {
				return chacha20.NewUnauthenticatedCipher(key, iv)
			},
		}
	})
	RegisterCipher("xchacha20", func() Cipher {
		return &StreamCipher{
			KeyLength: chacha20.KeySize,
			IVLength:  chacha20.NonceSizeX,
			EncryptConstructor: func(key []byte, iv []byte) (cipher.Stream, error) {
				return chacha20.NewUnauthenticatedCipher(key, iv)
			},
			DecryptConstructor: func(key []byte, iv []byte) (cipher.Stream, error) {
				return chacha20.NewUnauthenticatedCipher(key, iv)
			},
		}
	})
}
