//go:build arm64 || ppc64le || s390x

package shadowstream

import (
	"crypto/cipher"

	"golang.org/x/crypto/chacha20"
	"sing/protocol/shadowsocks"
)

func init() {
	shadowsocks.RegisterCipher("chacha20", func() shadowsocks.Cipher {
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
	shadowsocks.RegisterCipher("xchacha20", func() shadowsocks.Cipher {
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
