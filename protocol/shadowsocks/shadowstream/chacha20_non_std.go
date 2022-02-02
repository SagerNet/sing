//go:build !(arm64 || ppc64le || s390x)

package shadowstream

import (
	"crypto/cipher"

	"github.com/aead/chacha20"
	"github.com/aead/chacha20/chacha"
	"sing/protocol/shadowsocks"
)

func init() {
	shadowsocks.RegisterCipher("chacha20", func() shadowsocks.Cipher {
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
	shadowsocks.RegisterCipher("xchacha20", func() shadowsocks.Cipher {
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
