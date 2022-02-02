package shadowstream

import (
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
	"sing/common/buf"
	"sing/common/crypto"
	"sing/common/exceptions"
	"sing/protocol/shadowsocks"
)

func init() {
	shadowsocks.RegisterCipher("aes-128-ctr", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewCTR),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewCTR),
		}
	})
	shadowsocks.RegisterCipher("aes-192-ctr", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          24,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewCTR),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewCTR),
		}
	})
	shadowsocks.RegisterCipher("aes-256-ctr", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          32,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewCTR),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewCTR),
		}
	})

	shadowsocks.RegisterCipher("aes-128-cfb", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewCFBDecrypter),
		}
	})
	shadowsocks.RegisterCipher("aes-192-cfb", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          24,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewCFBDecrypter),
		}
	})
	shadowsocks.RegisterCipher("aes-256-cfb", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          32,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewCFBDecrypter),
		}
	})

	shadowsocks.RegisterCipher("aes-128-cfb8", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cfb8.NewEncrypter),
			DecryptConstructor: blockStream(aes.NewCipher, cfb8.NewDecrypter),
		}
	})
	shadowsocks.RegisterCipher("aes-192-cfb8", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          24,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cfb8.NewEncrypter),
			DecryptConstructor: blockStream(aes.NewCipher, cfb8.NewDecrypter),
		}
	})
	shadowsocks.RegisterCipher("aes-256-cfb8", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          32,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cfb8.NewEncrypter),
			DecryptConstructor: blockStream(aes.NewCipher, cfb8.NewDecrypter),
		}
	})

	shadowsocks.RegisterCipher("aes-128-ofb", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewOFB),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewOFB),
		}
	})
	shadowsocks.RegisterCipher("aes-192-ofb", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          24,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewOFB),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewOFB),
		}
	})
	shadowsocks.RegisterCipher("aes-256-ofb", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          32,
			IVLength:           aes.BlockSize,
			EncryptConstructor: blockStream(aes.NewCipher, cipher.NewOFB),
			DecryptConstructor: blockStream(aes.NewCipher, cipher.NewOFB),
		}
	})

	shadowsocks.RegisterCipher("rc4", func() shadowsocks.Cipher {
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
	shadowsocks.RegisterCipher("rc4-md5", func() shadowsocks.Cipher {
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

	shadowsocks.RegisterCipher("bf-cfb", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           blowfish.BlockSize,
			EncryptConstructor: blockStream(func(key []byte) (cipher.Block, error) { return blowfish.NewCipher(key) }, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(func(key []byte) (cipher.Block, error) { return blowfish.NewCipher(key) }, cipher.NewCFBDecrypter),
		}
	})
	shadowsocks.RegisterCipher("cast5-cfb", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           cast5.BlockSize,
			EncryptConstructor: blockStream(func(key []byte) (cipher.Block, error) { return cast5.NewCipher(key) }, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(func(key []byte) (cipher.Block, error) { return cast5.NewCipher(key) }, cipher.NewCFBDecrypter),
		}
	})
	shadowsocks.RegisterCipher("des-cfb", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          8,
			IVLength:           des.BlockSize,
			EncryptConstructor: blockStream(des.NewCipher, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(des.NewCipher, cipher.NewCFBDecrypter),
		}
	})
	shadowsocks.RegisterCipher("idea-cfb", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           8,
			EncryptConstructor: blockStream(idea.NewCipher, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(idea.NewCipher, cipher.NewCFBDecrypter),
		}
	})
	shadowsocks.RegisterCipher("rc2-cfb", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           rc2.BlockSize,
			EncryptConstructor: blockStream(func(key []byte) (cipher.Block, error) { return rc2.New(key, 16) }, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(func(key []byte) (cipher.Block, error) { return rc2.New(key, 16) }, cipher.NewCFBDecrypter),
		}
	})
	shadowsocks.RegisterCipher("seed-cfb", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           seed.BlockSize,
			EncryptConstructor: blockStream(seed.NewCipher, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(seed.NewCipher, cipher.NewCFBDecrypter),
		}
	})

	shadowsocks.RegisterCipher("camellia-128-cfb", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           camellia.BlockSize,
			EncryptConstructor: blockStream(camellia.New, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(camellia.New, cipher.NewCFBDecrypter),
		}
	})
	shadowsocks.RegisterCipher("camellia-192-cfb", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          24,
			IVLength:           camellia.BlockSize,
			EncryptConstructor: blockStream(camellia.New, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(camellia.New, cipher.NewCFBDecrypter),
		}
	})
	shadowsocks.RegisterCipher("camellia-256-cfb", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          32,
			IVLength:           camellia.BlockSize,
			EncryptConstructor: blockStream(camellia.New, cipher.NewCFBEncrypter),
			DecryptConstructor: blockStream(camellia.New, cipher.NewCFBDecrypter),
		}
	})

	shadowsocks.RegisterCipher("camellia-128-cfb8", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          16,
			IVLength:           camellia.BlockSize,
			EncryptConstructor: blockStream(camellia.New, cfb8.NewEncrypter),
			DecryptConstructor: blockStream(camellia.New, cfb8.NewDecrypter),
		}
	})
	shadowsocks.RegisterCipher("camellia-192-cfb8", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          24,
			IVLength:           camellia.BlockSize,
			EncryptConstructor: blockStream(camellia.New, cfb8.NewEncrypter),
			DecryptConstructor: blockStream(camellia.New, cfb8.NewDecrypter),
		}
	})
	shadowsocks.RegisterCipher("camellia-256-cfb8", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          32,
			IVLength:           camellia.BlockSize,
			EncryptConstructor: blockStream(camellia.New, cfb8.NewEncrypter),
			DecryptConstructor: blockStream(camellia.New, cfb8.NewDecrypter),
		}
	})

	shadowsocks.RegisterCipher("salsa20", func() shadowsocks.Cipher {
		return &StreamCipher{
			KeyLength:          32,
			IVLength:           8,
			EncryptConstructor: crypto.NewSalsa20,
			DecryptConstructor: crypto.NewSalsa20,
		}
	})

	shadowsocks.RegisterCipher("chacha20-ietf", func() shadowsocks.Cipher {
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

func (s *StreamCipher) EncodePacket(key []byte, buffer *buf.Buffer) error {
	iv := buffer.To(s.IVLength)
	streamCipher, err := s.EncryptConstructor(key, iv)
	if err != nil {
		return err
	}
	data := buffer.From(s.IVLength)
	streamCipher.XORKeyStream(data, data)
	return nil
}

func (s *StreamCipher) DecodePacket(key []byte, buffer *buf.Buffer) error {
	if buffer.Len() <= s.IVLength {
		return exceptions.New("insufficient data: ", buffer.Len())
	}
	iv := buffer.From(s.IVLength)
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

func (r *StreamReader) Upstream() io.Reader {
	return r.upstream
}

func (r *StreamReader) Process(p []byte, readN int) (n int, err error) {
	n = readN
	if n > 0 {
		r.cipher.XORKeyStream(p[:n], p[:n])
	}
	return
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

func (w *StreamWriter) Upstream() io.Writer {
	return w.upstream
}

func (w *StreamWriter) Process(p []byte) (n int, buffer *buf.Buffer, flush bool, err error) {
	w.cipher.XORKeyStream(p, p)
	n = len(p)
	return
}

func (w *StreamWriter) Write(p []byte) (n int, err error) {
	w.cipher.XORKeyStream(p, p)
	return w.upstream.Write(p)
}
