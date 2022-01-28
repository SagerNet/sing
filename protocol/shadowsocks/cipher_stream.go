//go:build !no_shadowsocks_stream

package shadowsocks

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/md5"
	"crypto/rc4"
	"io"
	"sync"

	"github.com/aead/chacha20"
	"github.com/aead/chacha20/chacha"
	"github.com/dgryski/go-camellia"
	"github.com/dgryski/go-idea"
	"github.com/dgryski/go-rc2"
	"github.com/geeksbaek/seed"
	"github.com/kierdavis/cfb8"
	"golang.org/x/crypto/blowfish"
	"golang.org/x/crypto/cast5"
	"sing/common/crypto"
	"sing/common/exceptions"
)

func init() {
	RegisterCipher("aes-128-ctr", func() Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewCTR),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewCTR),
		}
	})
	RegisterCipher("aes-192-ctr", func() Cipher {
		return &StreamCipher{
			KeyLength:          24,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewCTR),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewCTR),
		}
	})
	RegisterCipher("aes-256-ctr", func() Cipher {
		return &StreamCipher{
			KeyLength:          32,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewCTR),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewCTR),
		}
	})

	RegisterCipher("aes-128-cfb", func() Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewCFBDecrypter),
		}
	})
	RegisterCipher("aes-192-cfb", func() Cipher {
		return &StreamCipher{
			KeyLength:          24,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewCFBDecrypter),
		}
	})
	RegisterCipher("aes-256-cfb", func() Cipher {
		return &StreamCipher{
			KeyLength:          32,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewCFBDecrypter),
		}
	})

	RegisterCipher("aes-128-cfb8", func() Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cfb8.NewEncrypter),
			DecryptConstructor: blockStream(aes.NewCipher, cfb8.NewDecrypter),
		}
	})
	RegisterCipher("aes-192-cfb8", func() Cipher {
		return &StreamCipher{
			KeyLength:          24,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cfb8.NewEncrypter),
			DecryptConstructor: blockStream(aes.NewCipher, cfb8.NewDecrypter),
		}
	})
	RegisterCipher("aes-256-cfb8", func() Cipher {
		return &StreamCipher{
			KeyLength:          32,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cfb8.NewEncrypter),
			DecryptConstructor: blockStream(aes.NewCipher, cfb8.NewDecrypter),
		}
	})

	RegisterCipher("aes-128-ofb", func() Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewOFB),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewOFB),
		}
	})
	RegisterCipher("aes-192-ofb", func() Cipher {
		return &StreamCipher{
			KeyLength:          24,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewOFB),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewOFB),
		}
	})
	RegisterCipher("aes-256-ofb", func() Cipher {
		return &StreamCipher{
			KeyLength:          32,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewOFB),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewOFB),
		}
	})

	RegisterCipher("rc4", func() Cipher {
		return &StreamCipher{
			KeyLength: 16,
			IVLength:  16,
			EncryptConstructor: func(key []byte, iv []byte) (cipher.Stream, error) {
				return rc4.NewCipher(key)
			},
			DecryptConstructor: func(key []byte, iv []byte) (cipher.Stream, error) {
				return rc4.NewCipher(key)
			},
		}
	})
	RegisterCipher("rc4-md5", func() Cipher {
		return &StreamCipher{
			KeyLength: 16,
			IVLength:  16,
			EncryptConstructor: func(key []byte, iv []byte) (cipher.Stream, error) {
				h := md5.New()
				h.Write(key)
				h.Write(iv)
				return rc4.NewCipher(h.Sum(nil))
			},
			DecryptConstructor: func(key []byte, iv []byte) (cipher.Stream, error) {
				h := md5.New()
				h.Write(key)
				h.Write(iv)
				return rc4.NewCipher(h.Sum(nil))
			},
		}
	})

	RegisterCipher("bf-cfb", func() Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           blowfish.BlockSize,
			EncryptConstructor: blockStream(func(key []byte) (cipher.Block, error) { return blowfish.NewCipher(key) }, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(func(key []byte) (cipher.Block, error) { return blowfish.NewCipher(key) }, cipher.NewCFBDecrypter),
		}
	})
	RegisterCipher("cast5-cfb", func() Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           cast5.BlockSize,
			EncryptConstructor: blockStream(func(key []byte) (cipher.Block, error) { return cast5.NewCipher(key) }, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(func(key []byte) (cipher.Block, error) { return cast5.NewCipher(key) }, cipher.NewCFBDecrypter),
		}
	})
	RegisterCipher("des-cfb", func() Cipher {
		return &StreamCipher{
			KeyLength:          8,
			IVLength:           des.BlockSize,
			EncryptConstructor: blockStream(des.NewCipher, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(des.NewCipher, cipher.NewCFBDecrypter),
		}
	})
	RegisterCipher("idea-cfb", func() Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           8,
			EncryptConstructor: blockStream(idea.NewCipher, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(idea.NewCipher, cipher.NewCFBDecrypter),
		}
	})
	RegisterCipher("rc2-cfb", func() Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           rc2.BlockSize,
			EncryptConstructor: blockStream(func(key []byte) (cipher.Block, error) { return rc2.New(key, 16) }, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(func(key []byte) (cipher.Block, error) { return rc2.New(key, 16) }, cipher.NewCFBDecrypter),
		}
	})
	RegisterCipher("seed-cfb", func() Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           seed.BlockSize,
			EncryptConstructor: blockStream(seed.NewCipher, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(seed.NewCipher, cipher.NewCFBDecrypter),
		}
	})

	RegisterCipher("camellia-128-cfb", func() Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           camellia.BlockSize,
			EncryptConstructor: blockStream(camellia.New, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(camellia.New, cipher.NewCFBDecrypter),
		}
	})
	RegisterCipher("camellia-192-cfb", func() Cipher {
		return &StreamCipher{
			KeyLength:          24,
			IVLength:           camellia.BlockSize,
			EncryptConstructor: blockStream(camellia.New, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(camellia.New, cipher.NewCFBDecrypter),
		}
	})
	RegisterCipher("camellia-256-cfb", func() Cipher {
		return &StreamCipher{
			KeyLength:          32,
			IVLength:           camellia.BlockSize,
			EncryptConstructor: blockStream(camellia.New, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(camellia.New, cipher.NewCFBDecrypter),
		}
	})

	RegisterCipher("camellia-128-cfb8", func() Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           camellia.BlockSize,
			EncryptConstructor: blockStream(camellia.New, cfb8.NewEncrypter),
			DecryptConstructor: blockStream(camellia.New, cfb8.NewDecrypter),
		}
	})
	RegisterCipher("camellia-192-cfb8", func() Cipher {
		return &StreamCipher{
			KeyLength:          24,
			IVLength:           camellia.BlockSize,
			EncryptConstructor: blockStream(camellia.New, cfb8.NewEncrypter),
			DecryptConstructor: blockStream(camellia.New, cfb8.NewDecrypter),
		}
	})
	RegisterCipher("camellia-256-cfb8", func() Cipher {
		return &StreamCipher{
			KeyLength:          32,
			IVLength:           camellia.BlockSize,
			EncryptConstructor: blockStream(camellia.New, cfb8.NewEncrypter),
			DecryptConstructor: blockStream(camellia.New, cfb8.NewDecrypter),
		}
	})

	RegisterCipher("salsa20", func() Cipher {
		return &StreamCipher{
			KeyLength:          32,
			IVLength:           8,
			EncryptConstructor: crypto.NewSalsa20,
			DecryptConstructor: crypto.NewSalsa20,
		}
	})

	RegisterCipher("chacha20-ietf", func() Cipher {
		return &StreamCipher{
			KeyLength: chacha.KeySize,
			IVLength:  chacha.INonceSize,
			EncryptConstructor: func(key []byte, iv []byte) (cipher.Stream, error) {
				return chacha20.NewCipher(iv, key)
			},
			DecryptConstructor: func(key []byte, iv []byte) (cipher.Stream, error) {
				return chacha20.NewCipher(iv, key)
			},
		}
	})
}

func blockStream(blockCreator func(key []byte) (cipher.Block, error), streamCreator func(block cipher.Block, iv []byte) cipher.Stream) func([]byte, []byte) (cipher.Stream, error) {
	return func(key []byte, iv []byte) (cipher.Stream, error) {
		block, err := blockCreator(key)
		if err != nil {
			return nil, err
		}
		return streamCreator(block, iv), err
	}
}

type StreamCipher struct {
	KeyLength          int
	IVLength           int
	EncryptConstructor func(key []byte, iv []byte) (cipher.Stream, error)
	DecryptConstructor func(key []byte, iv []byte) (cipher.Stream, error)
	sync.Mutex
}

func (s *StreamCipher) KeySize() int {
	return s.KeyLength
}

func (s *StreamCipher) IVSize() int {
	return s.IVLength
}

func (s *StreamCipher) NewEncryptionWriter(key []byte, iv []byte, writer io.Writer) (io.Writer, error) {
	streamCipher, err := s.EncryptConstructor(key, iv)
	if err != nil {
		return nil, err
	}
	return &StreamWriter{writer, streamCipher}, nil
}

func (s *StreamCipher) NewDecryptionReader(key []byte, iv []byte, reader io.Reader) (io.Reader, error) {
	streamCipher, err := s.DecryptConstructor(key, iv)
	if err != nil {
		return nil, err
	}
	return &StreamReader{reader, streamCipher}, nil
}

func (s *StreamCipher) EncodePacket(key []byte, buffer *bytes.Buffer) error {
	iv := buffer.Bytes()[:s.IVLength]
	streamCipher, err := s.EncryptConstructor(key, iv)
	if err != nil {
		return err
	}
	data := buffer.Bytes()[s.IVLength:]
	streamCipher.XORKeyStream(data, data)
	return nil
}

func (s *StreamCipher) DecodePacket(key []byte, buffer *bytes.Buffer) error {
	if buffer.Len() <= s.IVLength {
		return exceptions.New("insufficient data: ", buffer.Len())
	}
	iv := buffer.Bytes()[:s.IVLength]
	streamCipher, err := s.DecryptConstructor(key, iv)
	if err != nil {
		return err
	}
	end := buffer.Len() - s.IVLength
	streamCipher.XORKeyStream(buffer.Bytes()[:end], buffer.Bytes()[s.IVLength:])
	buffer.Truncate(end)
	return nil
}

type StreamReader struct {
	upstream io.Reader
	cipher   cipher.Stream
}

func (r *StreamReader) Read(p []byte) (n int, err error) {
	n, err = r.upstream.Read(p)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		r.cipher.XORKeyStream(p[:n], p[:n])
	}
	return
}

type StreamWriter struct {
	upstream io.Writer
	cipher   cipher.Stream
}

func (w *StreamWriter) Write(p []byte) (n int, err error) {
	w.cipher.XORKeyStream(p, p)
	return w.upstream.Write(p)
}
