package shadowaead

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"io"
	"net"
	"sync"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/replay"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/protocol/shadowsocks"
	"github.com/sagernet/sing/protocol/socks"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

var List = []string{
	"aes-128-gcm",
	"aes-192-gcm",
	"aes-256-gcm",
	"chacha20-ietf-poly1305",
	"xchacha20-ietf-poly1305",
}

var (
	ErrBadKey          = E.New("bad key")
	ErrMissingPassword = E.New("missing password")
)

func New(method string, key []byte, password []byte, secureRNG io.Reader, replayFilter bool) (shadowsocks.Method, error) {
	m := &Method{
		name:      method,
		secureRNG: secureRNG,
	}
	if replayFilter {
		m.replayFilter = replay.NewBloomRing()
	}
	switch method {
	case "aes-128-gcm":
		m.keySaltLength = 16
		m.constructor = newAESGCM
	case "aes-192-gcm":
		m.keySaltLength = 24
		m.constructor = newAESGCM
	case "aes-256-gcm":
		m.keySaltLength = 32
		m.constructor = newAESGCM
	case "chacha20-ietf-poly1305":
		m.keySaltLength = 32
		m.constructor = func(key []byte) cipher.AEAD {
			cipher, err := chacha20poly1305.New(key)
			common.Must(err)
			return cipher
		}
	case "xchacha20-ietf-poly1305":
		m.keySaltLength = 32
		m.constructor = func(key []byte) cipher.AEAD {
			cipher, err := chacha20poly1305.NewX(key)
			common.Must(err)
			return cipher
		}
	}
	if len(key) == m.keySaltLength {
		m.key = key
	} else if len(key) > 0 {
		return nil, ErrBadKey
	} else if len(password) > 0 {
		m.key = shadowsocks.Key(password, m.keySaltLength)
	} else {
		return nil, ErrMissingPassword
	}
	return m, nil
}

func Kdf(key, iv []byte, keyLength int) []byte {
	subKey := buf.Make(keyLength)
	kdf := hkdf.New(sha1.New, key, iv, []byte("ss-subkey"))
	common.Must1(io.ReadFull(kdf, common.Dup(subKey)))
	return subKey
}

func newAESGCM(key []byte) cipher.AEAD {
	block, err := aes.NewCipher(key)
	common.Must(err)
	aead, err := cipher.NewGCM(block)
	common.Must(err)
	return aead
}

type Method struct {
	name          string
	keySaltLength int
	constructor   func(key []byte) cipher.AEAD
	key           []byte
	secureRNG     io.Reader
	replayFilter  replay.Filter
}

func (m *Method) Name() string {
	return m.name
}

func (m *Method) KeyLength() int {
	return m.keySaltLength
}

func (m *Method) ReadRequest(upstream io.Reader) (io.Reader, error) {
	_salt := buf.Make(m.keySaltLength)
	salt := common.Dup(_salt)
	_, err := io.ReadFull(upstream, salt)
	if err != nil {
		return nil, E.Cause(err, "read salt")
	}
	if m.replayFilter != nil {
		if !m.replayFilter.Check(salt) {
			return nil, E.New("salt not unique")
		}
	}
	key := Kdf(m.key, salt, m.keySaltLength)
	return NewReader(upstream, m.constructor(common.Dup(key)), MaxPacketSize), nil
}

func (m *Method) WriteResponse(upstream io.Writer) (io.Writer, error) {
	_salt := buf.Make(m.keySaltLength)
	salt := common.Dup(_salt)
	common.Must1(io.ReadFull(m.secureRNG, salt))
	_, err := upstream.Write(salt)
	if err != nil {
		return nil, err
	}
	key := Kdf(m.key, salt, m.keySaltLength)
	return NewWriter(upstream, m.constructor(common.Dup(key)), MaxPacketSize), nil
}

func (m *Method) DialConn(conn net.Conn, destination *M.AddrPort) (net.Conn, error) {
	shadowsocksConn := &clientConn{
		Conn:        conn,
		method:      m,
		destination: destination,
	}
	return shadowsocksConn, shadowsocksConn.writeRequest(nil)
}

func (m *Method) DialEarlyConn(conn net.Conn, destination *M.AddrPort) net.Conn {
	return &clientConn{
		Conn:        conn,
		method:      m,
		destination: destination,
	}
}

func (m *Method) DialPacketConn(conn net.Conn) socks.PacketConn {
	return &clientPacketConn{conn, m}
}

func (m *Method) EncodePacket(buffer *buf.Buffer) error {
	key := Kdf(m.key, buffer.To(m.keySaltLength), m.keySaltLength)
	c := m.constructor(common.Dup(key))
	c.Seal(buffer.From(m.keySaltLength)[:0], rw.ZeroBytes[:c.NonceSize()], buffer.From(m.keySaltLength), nil)
	buffer.Extend(c.Overhead())
	return nil
}

func (m *Method) DecodePacket(buffer *buf.Buffer) error {
	if buffer.Len() < m.keySaltLength {
		return E.New("bad packet")
	}
	key := Kdf(m.key, buffer.To(m.keySaltLength), m.keySaltLength)
	c := m.constructor(common.Dup(key))
	packet, err := c.Open(buffer.Index(m.keySaltLength), rw.ZeroBytes[:c.NonceSize()], buffer.From(m.keySaltLength), nil)
	if err != nil {
		return err
	}
	buffer.Advance(m.keySaltLength)
	buffer.Truncate(len(packet))
	return nil
}

type clientConn struct {
	net.Conn

	method      *Method
	destination *M.AddrPort

	access sync.Mutex
	reader *Reader
	writer *Writer
}

func (c *clientConn) writeRequest(payload []byte) error {
	_salt := make([]byte, c.method.keySaltLength)
	salt := common.Dup(_salt)
	common.Must1(io.ReadFull(c.method.secureRNG, salt))

	key := Kdf(c.method.key, salt, c.method.keySaltLength)
	writer := NewWriter(
		c.Conn,
		c.method.constructor(common.Dup(key)),
		MaxPacketSize,
	)
	header := writer.Buffer()
	header.Write(salt)
	bufferedWriter := writer.BufferedWriter(header.Len())

	if len(payload) > 0 {
		err := socks.AddressSerializer.WriteAddrPort(bufferedWriter, c.destination)
		if err != nil {
			return err
		}

		_, err = bufferedWriter.Write(payload)
		if err != nil {
			return err
		}
	} else {
		err := socks.AddressSerializer.WriteAddrPort(bufferedWriter, c.destination)
		if err != nil {
			return err
		}
	}

	err := bufferedWriter.Flush()
	if err != nil {
		return err
	}

	c.writer = writer
	return nil
}

func (c *clientConn) readResponse() error {
	if c.reader != nil {
		return nil
	}
	_salt := buf.Make(c.method.keySaltLength)
	salt := common.Dup(_salt)
	_, err := io.ReadFull(c.Conn, salt)
	if err != nil {
		return err
	}
	if c.method.replayFilter != nil {
		if !c.method.replayFilter.Check(salt) {
			return E.New("salt not unique")
		}
	}
	key := Kdf(c.method.key, salt, c.method.keySaltLength)
	c.reader = NewReader(
		c.Conn,
		c.method.constructor(common.Dup(key)),
		MaxPacketSize,
	)
	return nil
}

func (c *clientConn) Read(p []byte) (n int, err error) {
	if err = c.readResponse(); err != nil {
		return
	}
	return c.reader.Read(p)
}

func (c *clientConn) WriteTo(w io.Writer) (n int64, err error) {
	if err = c.readResponse(); err != nil {
		return
	}
	return c.reader.WriteTo(w)
}

func (c *clientConn) Write(p []byte) (n int, err error) {
	if c.writer != nil {
		return c.writer.Write(p)
	}

	c.access.Lock()

	if c.writer != nil {
		c.access.Unlock()
		return c.writer.Write(p)
	}

	err = c.writeRequest(p)
	if err != nil {
		return
	}
	return len(p), nil
}

func (c *clientConn) ReadFrom(r io.Reader) (n int64, err error) {
	if c.writer == nil {
		return rw.ReadFrom0(c, r)
	}
	return c.writer.ReadFrom(r)
}

type clientPacketConn struct {
	net.Conn
	method *Method
}

func (c *clientPacketConn) WritePacket(buffer *buf.Buffer, destination *M.AddrPort) error {
	_header := buf.StackNew()
	header := common.Dup(_header)
	common.Must1(header.ReadFullFrom(c.method.secureRNG, c.method.keySaltLength))
	err := socks.AddressSerializer.WriteAddrPort(header, destination)
	if err != nil {
		return err
	}
	buffer = buffer.WriteBufferAtFirst(header)
	err = c.method.EncodePacket(buffer)
	if err != nil {
		return err
	}
	return common.Error(c.Write(buffer.Bytes()))
}

func (c *clientPacketConn) ReadPacket(buffer *buf.Buffer) (*M.AddrPort, error) {
	n, err := c.Read(buffer.FreeBytes())
	if err != nil {
		return nil, err
	}
	buffer.Truncate(n)
	err = c.method.DecodePacket(buffer)
	if err != nil {
		return nil, err
	}
	return socks.AddressSerializer.ReadAddrPort(buffer)
}
