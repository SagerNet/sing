package shadowsocks

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"io"
	"net"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/rw"
	"golang.org/x/crypto/chacha20poly1305"
)

const PacketLengthBufferSize = 2

func init() {
	RegisterCipher("aes-128-gcm", func() Cipher {
		return &AEADCipher{
			KeyLength:   16,
			SaltLength:  16,
			Constructor: aesGcm,
		}
	})
	RegisterCipher("aes-192-gcm", func() Cipher {
		return &AEADCipher{
			KeyLength:   24,
			SaltLength:  24,
			Constructor: aesGcm,
		}
	})
	RegisterCipher("aes-256-gcm", func() Cipher {
		return &AEADCipher{
			KeyLength:   32,
			SaltLength:  32,
			Constructor: aesGcm,
		}
	})
	RegisterCipher("chacha20-ietf-poly1305", func() Cipher {
		return &AEADCipher{
			KeyLength:   32,
			SaltLength:  32,
			Constructor: chacha20Poly1305,
		}
	})
	RegisterCipher("xchacha20-ietf-poly1305", func() Cipher {
		return &AEADCipher{
			KeyLength:   32,
			SaltLength:  32,
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
	SaltLength  int
	Constructor func(key []byte) cipher.AEAD
}

func (c *AEADCipher) KeySize() int {
	return c.KeyLength
}

func (c *AEADCipher) SaltSize() int {
	return c.SaltLength
}

func (c *AEADCipher) CreateReader(key []byte, salt []byte, reader io.Reader) io.Reader {
	return NewAEADReader(reader, c.Constructor(Kdf(key, salt, c.KeyLength)))
}

func (c *AEADCipher) CreateWriter(key []byte, salt []byte, writer io.Writer) (io.Writer, int) {
	protocolWriter := NewAEADWriter(writer, c.Constructor(Kdf(key, salt, c.KeyLength)))
	return protocolWriter, protocolWriter.maxDataSize
}

func (c *AEADCipher) EncodePacket(key []byte, buffer *buf.Buffer) error {
	aead := c.Constructor(Kdf(key, buffer.To(c.SaltLength), c.KeyLength))
	aead.Seal(buffer.From(c.SaltLength)[:0], rw.ZeroBytes[:aead.NonceSize()], buffer.From(c.SaltLength), nil)
	buffer.Extend(aead.Overhead())
	return nil
}

func (c *AEADCipher) DecodePacket(key []byte, buffer *buf.Buffer) error {
	if buffer.Len() < c.SaltLength {
		return exceptions.New("bad packet")
	}
	aead := c.Constructor(Kdf(key, buffer.To(c.SaltLength), c.KeyLength))
	packet, err := aead.Open(buffer.Index(c.SaltLength), rw.ZeroBytes[:aead.NonceSize()], buffer.From(c.SaltLength), nil)
	if err != nil {
		return err
	}
	buffer.Advance(c.SaltLength)
	buffer.Truncate(len(packet))
	return nil
}

type AEADConn struct {
	net.Conn
	Reader *AEADReader
	Writer *AEADWriter
}

func (c *AEADConn) Read(p []byte) (n int, err error) {
	return c.Reader.Read(p)
}

func (c *AEADConn) Write(p []byte) (n int, err error) {
	return c.Writer.Write(p)
}

func (c *AEADConn) Close() error {
	c.Reader.Close()
	c.Writer.Close()
	return c.Conn.Close()
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

		_, err = w.upstream.Write(packet)
		if err != nil {
			return
		}
		n += len(data)
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
