package shadowsocks

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"sing/common"
	"sing/common/buf"
	"sing/common/exceptions"
	"sing/common/rw"
)

func init() {
	RegisterCipher("aes-128-gcm", func() Cipher {
		return &AEADCipher{
			KeyLength:   16,
			IVLength:    16,
			Constructor: aesGcm,
		}
	})
	RegisterCipher("aes-192-gcm", func() Cipher {
		return &AEADCipher{
			KeyLength:   24,
			IVLength:    24,
			Constructor: aesGcm,
		}
	})
	RegisterCipher("aes-256-gcm", func() Cipher {
		return &AEADCipher{
			KeyLength:   32,
			IVLength:    32,
			Constructor: aesGcm,
		}
	})
	RegisterCipher("chacha20-ietf-poly1305", func() Cipher {
		return &AEADCipher{
			KeyLength:   32,
			IVLength:    32,
			Constructor: chacha20Poly1305,
		}
	})
	RegisterCipher("xchacha20-ietf-poly1305", func() Cipher {
		return &AEADCipher{
			KeyLength:   32,
			IVLength:    32,
			Constructor: xchacha20Poly1305,
		}
	})
}

func aesGcm(key []byte) cipher.AEAD {
	block, err := aes.NewCipher(key)
	common.Must(err)
	aead, err := cipher.NewGCM(block)
	common.Must(err)
	return aead
}

func chacha20Poly1305(key []byte) cipher.AEAD {
	aead, err := chacha20poly1305.New(key)
	common.Must(err)
	return aead
}

func xchacha20Poly1305(key []byte) cipher.AEAD {
	aead, err := chacha20poly1305.NewX(key)
	common.Must(err)
	return aead
}

type AEADCipher struct {
	KeyLength   int
	IVLength    int
	Constructor func(key []byte) cipher.AEAD
}

func (c *AEADCipher) KeySize() int {
	return c.KeyLength
}

func (c *AEADCipher) IVSize() int {
	return c.IVLength
}

func (c *AEADCipher) NewEncryptionWriter(key []byte, iv []byte, writer io.Writer) (io.Writer, error) {
	return NewAEADWriter(writer, c.Constructor(Kdf(key, iv, c.KeyLength))), nil
}

func (c *AEADCipher) NewDecryptionReader(key []byte, iv []byte, reader io.Reader) (io.Reader, error) {
	return NewAEADReader(reader, c.Constructor(Kdf(key, iv, c.KeyLength))), nil
}

func (c *AEADCipher) EncodePacket(key []byte, buffer *bytes.Buffer) error {
	aead := c.Constructor(Kdf(key, buffer.Bytes()[:c.IVLength], c.KeyLength))
	end := buffer.Len()
	buffer.Grow(aead.Overhead())
	aead.Seal(buffer.Bytes()[:c.IVLength], rw.ZeroBytes[:aead.NonceSize()], buffer.Bytes()[c.IVLength:end], nil)
	return nil
}

func (c *AEADCipher) DecodePacket(key []byte, buffer *bytes.Buffer) error {
	if buffer.Len() < c.IVLength {
		return exceptions.New("bad packet")
	}
	aead := c.Constructor(Kdf(key, buffer.Bytes()[:c.IVLength], c.KeyLength))
	_, err := aead.Open(buffer.Bytes()[:c.IVLength], rw.ZeroBytes[:aead.NonceSize()], buffer.Bytes()[c.IVLength:], nil)
	if err != nil {
		return err
	}
	buffer.Truncate(aead.Overhead())
	return nil
}

type AEADReader struct {
	upstream io.Reader
	cipher   cipher.AEAD
	buffer   *bytes.Buffer
	data     []byte
	nonce    []byte
	index    int
	cached   int
}

func NewAEADReader(upstream io.Reader, cipher cipher.AEAD) *AEADReader {
	buffer := buf.New()
	buffer.Grow(MaxPacketSize)
	return &AEADReader{
		upstream: upstream,
		cipher:   cipher,
		buffer:   buffer,
		data:     buffer.Bytes(),
		nonce:    make([]byte, cipher.NonceSize()),
	}
}

func (r *AEADReader) Read(b []byte) (n int, err error) {
	if r.cached > 0 {
		n = copy(b, r.data[r.index:r.index+r.cached])
		r.cached -= n
		r.index += n
		return
	}
	start := PacketLengthBufferSize + r.cipher.Overhead()
	_, err = io.ReadFull(r.upstream, r.data[:start])
	if err != nil {
		return 0, err
	}
	_, err = r.cipher.Open(r.data[:0], r.nonce, r.data[:start], nil)
	if err != nil {
		return 0, err
	}
	increaseNonce(r.nonce)
	length := int(binary.BigEndian.Uint16(r.data[:PacketLengthBufferSize]))
	end := length + r.cipher.Overhead()

	if len(b) >= end {
		data := b[:end]
		_, err = io.ReadFull(r.upstream, data)
		if err != nil {
			return 0, err
		}
		_, err = r.cipher.Open(b[:0], r.nonce, data, nil)
		if err != nil {
			return 0, err
		}
		increaseNonce(r.nonce)
		return length, nil
	} else {
		_, err = io.ReadFull(r.upstream, r.data[:end])
		if err != nil {
			return 0, err
		}
		_, err = r.cipher.Open(r.data[:0], r.nonce, r.data[:end], nil)
		if err != nil {
			return 0, err
		}
		increaseNonce(r.nonce)
		n = copy(b, r.data[:length])
		r.cached = length - n
		r.index = n
		return
	}
}

func (r *AEADReader) Close() error {
	buf.Release(r.buffer)
	return nil
}

type AEADWriter struct {
	upstream io.Writer
	cipher   cipher.AEAD
	buffer   *bytes.Buffer
	data     []byte
	nonce    []byte
}

func NewAEADWriter(upstream io.Writer, cipher cipher.AEAD) *AEADWriter {
	buffer := buf.New()
	buffer.Grow(MaxPacketSize)
	return &AEADWriter{
		upstream: upstream,
		cipher:   cipher,
		buffer:   buffer,
		data:     buffer.Bytes(),
		nonce:    make([]byte, cipher.NonceSize()),
	}
}

func (w *AEADWriter) Write(p []byte) (n int, err error) {
	maxDataSize := MaxPacketSize - PacketLengthBufferSize - w.cipher.Overhead()*2

	for _, data := range buf.ForeachN(p, maxDataSize) {

		binary.BigEndian.PutUint16(w.data[:PacketLengthBufferSize], uint16(len(data)))
		w.cipher.Seal(w.data[:0], w.nonce, w.data[:PacketLengthBufferSize], nil)
		increaseNonce(w.nonce)

		start := w.cipher.Overhead() + PacketLengthBufferSize
		packet := w.cipher.Seal(w.data[:start], w.nonce, data, nil)
		increaseNonce(w.nonce)

		pn, err := w.upstream.Write(packet)
		if err != nil {
			return 0, err
		}
		n += pn
	}

	return
}

func (w *AEADWriter) Close() error {
	buf.Release(w.buffer)
	return nil
}

func increaseNonce(nonce []byte) {
	for i := range nonce {
		nonce[i]++
		if nonce[i] != 0 {
			return
		}
	}
}
