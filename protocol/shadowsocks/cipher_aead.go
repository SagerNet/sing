package shadowsocks

import (
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

const PacketLengthBufferSize = 2

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

func (c *AEADCipher) EncodePacket(key []byte, buffer *buf.Buffer) error {
	aead := c.Constructor(Kdf(key, buffer.To(c.IVLength), c.KeyLength))
	aead.Seal(buffer.From(c.IVLength)[:0], rw.ZeroBytes[:aead.NonceSize()], buffer.From(c.IVLength), nil)
	buffer.Extend(aead.Overhead())
	return nil
}

func (c *AEADCipher) DecodePacket(key []byte, buffer *buf.Buffer) error {
	if buffer.Len() < c.IVLength {
		return exceptions.New("bad packet")
	}
	aead := c.Constructor(Kdf(key, buffer.To(c.IVLength), c.KeyLength))
	packet, err := aead.Open(buffer.Index(0), rw.ZeroBytes[:aead.NonceSize()], buffer.From(c.IVLength), nil)
	if err != nil {
		return err
	}
	buffer.Truncate(len(packet))
	return nil
}

type AEADReader struct {
	upstream io.Reader
	cipher   cipher.AEAD
	data     []byte
	nonce    []byte
	index    int
	cached   int
}

func NewAEADReader(upstream io.Reader, cipher cipher.AEAD) *AEADReader {
	return &AEADReader{
		upstream: upstream,
		cipher:   cipher,
		data:     buf.GetBytes(),
		nonce:    make([]byte, cipher.NonceSize()),
	}
}

func (r *AEADReader) Upstream() io.Reader {
	return r.upstream
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
	if r.data != nil {
		buf.PutBytes(r.data)
		r.data = nil
	}
	return nil
}

type AEADWriter struct {
	upstream    io.Writer
	cipher      cipher.AEAD
	data        []byte
	nonce       []byte
	maxDataSize int
}

func NewAEADWriter(upstream io.Writer, cipher cipher.AEAD) *AEADWriter {
	return &AEADWriter{
		upstream:    upstream,
		cipher:      cipher,
		data:        buf.GetBytes(),
		nonce:       make([]byte, cipher.NonceSize()),
		maxDataSize: MaxPacketSize - PacketLengthBufferSize - cipher.Overhead()*2,
	}
}

func (w *AEADWriter) Upstream() io.Writer {
	return w.upstream
}

func (w *AEADWriter) Process(p []byte) (n int, buffer *buf.Buffer, flush bool, err error) {
	if len(p) > w.maxDataSize {
		n, err = w.Write(p)
		err = &rw.DirectException{
			Suppressed: err,
		}
		return
	}

	binary.BigEndian.PutUint16(w.data[:PacketLengthBufferSize], uint16(len(p)))
	encryptedLength := w.cipher.Seal(w.data[:0], w.nonce, w.data[:PacketLengthBufferSize], nil)
	increaseNonce(w.nonce)
	start := len(encryptedLength)

	/*
		no usage
		if cap(p) > len(p)+PacketLengthBufferSize+2*w.cipher.Overhead() {
			packet := w.cipher.Seal(p[:start], w.nonce, p, nil)
			increaseNonce(w.nonce)
			copy(p[:start], encryptedLength)
			n = start + len(packet)
			return
		}
	*/

	packet := w.cipher.Seal(w.data[:start], w.nonce, p, nil)
	increaseNonce(w.nonce)
	return 0, buf.As(packet), false, err
}

func (w *AEADWriter) Write(p []byte) (n int, err error) {
	for _, data := range buf.ForeachN(p, w.maxDataSize) {

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
	if w.data != nil {
		buf.PutBytes(w.data)
		w.data = nil
	}
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
