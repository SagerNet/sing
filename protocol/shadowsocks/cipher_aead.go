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

func (c *AEADCipher) CreateWriter(key []byte, salt []byte, writer io.Writer) io.Writer {
	protocolWriter := NewAEADWriter(writer, c.Constructor(Kdf(key, salt, c.KeyLength)))
	return protocolWriter
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
		data:     make([]byte, MaxPacketSize+PacketLengthBufferSize+cipher.Overhead()*2),
		nonce:    make([]byte, cipher.NonceSize()),
	}
}

func (r *AEADReader) Upstream() io.Reader {
	return r.upstream
}

func (r *AEADReader) Replaceable() bool {
	return false
}

func (r *AEADReader) SetUpstream(reader io.Reader) {
	r.upstream = reader
}

func (r *AEADReader) WriteTo(writer io.Writer) (n int64, err error) {
	if r.cached > 0 {
		writeN, writeErr := writer.Write(r.data[r.index : r.index+r.cached])
		if writeErr != nil {
			return int64(writeN), writeErr
		}
		n += int64(writeN)
	}
	for {
		start := PacketLengthBufferSize + r.cipher.Overhead()
		_, err = io.ReadFull(r.upstream, r.data[:start])
		if err != nil {
			return
		}
		_, err = r.cipher.Open(r.data[:0], r.nonce, r.data[:start], nil)
		if err != nil {
			return
		}
		increaseNonce(r.nonce)
		length := int(binary.BigEndian.Uint16(r.data[:PacketLengthBufferSize]))
		end := length + r.cipher.Overhead()
		_, err = io.ReadFull(r.upstream, r.data[:end])
		if err != nil {
			return
		}
		_, err = r.cipher.Open(r.data[:0], r.nonce, r.data[:end], nil)
		if err != nil {
			return
		}
		increaseNonce(r.nonce)
		writeN, writeErr := writer.Write(r.data[:length])
		if writeErr != nil {
			return int64(writeN), writeErr
		}
		n += int64(writeN)
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

type AEADWriter struct {
	upstream io.Writer
	cipher   cipher.AEAD
	data     []byte
	nonce    []byte
}

func NewAEADWriter(upstream io.Writer, cipher cipher.AEAD) *AEADWriter {
	return &AEADWriter{
		upstream: upstream,
		cipher:   cipher,
		data:     make([]byte, MaxPacketSize+PacketLengthBufferSize+cipher.Overhead()*2),
		nonce:    make([]byte, cipher.NonceSize()),
	}
}

func (w *AEADWriter) Upstream() io.Writer {
	return w.upstream
}

func (w *AEADWriter) Replaceable() bool {
	return false
}

func (w *AEADWriter) SetWriter(writer io.Writer) {
	w.upstream = writer
}

func (w *AEADWriter) ReadFrom(r io.Reader) (n int64, err error) {
	for {
		offset := w.cipher.Overhead() + PacketLengthBufferSize
		readN, readErr := r.Read(w.data[offset : offset+MaxPacketSize])
		if readErr != nil {
			return 0, readErr
		}
		binary.BigEndian.PutUint16(w.data[:PacketLengthBufferSize], uint16(readN))
		w.cipher.Seal(w.data[:0], w.nonce, w.data[:PacketLengthBufferSize], nil)
		increaseNonce(w.nonce)
		packet := w.cipher.Seal(w.data[offset:offset], w.nonce, w.data[offset:offset+readN], nil)
		increaseNonce(w.nonce)
		_, err = w.upstream.Write(w.data[:offset+len(packet)])
		if err != nil {
			return
		}
		err = common.FlushVar(&w.upstream)
		if err != nil {
			return
		}
		n += int64(readN)
	}
}

func (w *AEADWriter) Write(p []byte) (n int, err error) {
	for _, data := range buf.ForeachN(p, MaxPacketSize) {
		binary.BigEndian.PutUint16(w.data[:PacketLengthBufferSize], uint16(len(data)))
		w.cipher.Seal(w.data[:0], w.nonce, w.data[:PacketLengthBufferSize], nil)
		increaseNonce(w.nonce)
		offset := w.cipher.Overhead() + PacketLengthBufferSize
		packet := w.cipher.Seal(w.data[offset:offset], w.nonce, data, nil)
		increaseNonce(w.nonce)
		_, err = w.upstream.Write(w.data[:offset+len(packet)])
		if err != nil {
			return
		}
		n += len(data)
	}

	return
}

func increaseNonce(nonce []byte) {
	for i := range nonce {
		nonce[i]++
		if nonce[i] != 0 {
			return
		}
	}
}
