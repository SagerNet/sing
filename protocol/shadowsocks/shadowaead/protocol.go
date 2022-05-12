package shadowaead

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"io"
	"net"
	"runtime"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/protocol/shadowsocks"
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

func New(method string, key []byte, password string, secureRNG io.Reader) (shadowsocks.Method, error) {
	m := &Method{
		name:      method,
		secureRNG: secureRNG,
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
		return nil, shadowsocks.ErrBadKey
	} else if password == "" {
		return nil, shadowsocks.ErrMissingPassword
	} else {
		m.key = shadowsocks.Key([]byte(password), m.keySaltLength)
	}
	return m, nil
}

func Kdf(key, iv []byte, keyLength int) []byte {
	info := []byte("ss-subkey")
	subKey := buf.Make(keyLength)
	kdf := hkdf.New(sha1.New, key, iv, common.Dup(info))
	runtime.KeepAlive(info)
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
}

func (m *Method) Name() string {
	return m.name
}

func (m *Method) KeyLength() int {
	return m.keySaltLength
}

func (m *Method) ReadRequest(upstream io.Reader) (io.Reader, error) {
	_salt := buf.Make(m.keySaltLength)
	defer runtime.KeepAlive(_salt)
	salt := common.Dup(_salt)
	_, err := io.ReadFull(upstream, salt)
	if err != nil {
		return nil, E.Cause(err, "read salt")
	}
	key := Kdf(m.key, salt, m.keySaltLength)
	defer runtime.KeepAlive(key)
	return NewReader(upstream, m.constructor(common.Dup(key)), MaxPacketSize), nil
}

func (m *Method) WriteResponse(upstream io.Writer) (io.Writer, error) {
	_salt := buf.Make(m.keySaltLength)
	defer runtime.KeepAlive(_salt)
	salt := common.Dup(_salt)
	common.Must1(io.ReadFull(m.secureRNG, salt))
	_, err := upstream.Write(salt)
	if err != nil {
		return nil, err
	}
	key := Kdf(m.key, salt, m.keySaltLength)
	return NewWriter(upstream, m.constructor(common.Dup(key)), MaxPacketSize), nil
}

func (m *Method) DialConn(conn net.Conn, destination M.Socksaddr) (net.Conn, error) {
	shadowsocksConn := &clientConn{
		Conn:        conn,
		method:      m,
		destination: destination,
	}
	return shadowsocksConn, shadowsocksConn.writeRequest(nil)
}

func (m *Method) DialEarlyConn(conn net.Conn, destination M.Socksaddr) net.Conn {
	return &clientConn{
		Conn:        conn,
		method:      m,
		destination: destination,
	}
}

func (m *Method) DialPacketConn(conn net.Conn) N.NetPacketConn {
	return &clientPacketConn{m, conn}
}

func (m *Method) EncodePacket(buffer *buf.Buffer) error {
	key := Kdf(m.key, buffer.To(m.keySaltLength), m.keySaltLength)
	c := m.constructor(common.Dup(key))
	runtime.KeepAlive(key)
	c.Seal(buffer.Index(m.keySaltLength), rw.ZeroBytes[:c.NonceSize()], buffer.From(m.keySaltLength), nil)
	buffer.Extend(c.Overhead())
	return nil
}

func (m *Method) DecodePacket(buffer *buf.Buffer) error {
	if buffer.Len() < m.keySaltLength {
		return E.New("bad packet")
	}
	key := Kdf(m.key, buffer.To(m.keySaltLength), m.keySaltLength)
	c := m.constructor(common.Dup(key))
	runtime.KeepAlive(key)
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
	destination M.Socksaddr
	reader      *Reader
	writer      *Writer
}

func (c *clientConn) writeRequest(payload []byte) error {
	_salt := make([]byte, c.method.keySaltLength)
	salt := common.Dup(_salt)
	common.Must1(io.ReadFull(c.method.secureRNG, salt))

	key := Kdf(c.method.key, salt, c.method.keySaltLength)
	runtime.KeepAlive(_salt)
	writer := NewWriter(
		c.Conn,
		c.method.constructor(common.Dup(key)),
		MaxPacketSize,
	)
	runtime.KeepAlive(key)
	header := writer.Buffer()
	header.Write(salt)
	bufferedWriter := writer.BufferedWriter(header.Len())

	if len(payload) > 0 {
		err := M.SocksaddrSerializer.WriteAddrPort(bufferedWriter, c.destination)
		if err != nil {
			return err
		}

		_, err = bufferedWriter.Write(payload)
		if err != nil {
			return err
		}
	} else {
		err := M.SocksaddrSerializer.WriteAddrPort(bufferedWriter, c.destination)
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
	defer runtime.KeepAlive(_salt)
	salt := common.Dup(_salt)
	_, err := io.ReadFull(c.Conn, salt)
	if err != nil {
		return err
	}
	key := Kdf(c.method.key, salt, c.method.keySaltLength)
	defer runtime.KeepAlive(key)
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

func (c *clientConn) UpstreamReader() io.Reader {
	if c.reader == nil {
		return c.Conn
	}
	return c.reader
}

func (c *clientConn) ReaderReplaceable() bool {
	return c.reader != nil
}

func (c *clientConn) UpstreamWriter() io.Writer {
	if c.writer == nil {
		return c.Conn
	}
	return c.writer
}

func (c *clientConn) WriterReplaceable() bool {
	return c.writer != nil
}

type clientPacketConn struct {
	*Method
	net.Conn
}

func (c *clientPacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	header := buffer.ExtendHeader(c.keySaltLength + M.SocksaddrSerializer.AddrPortLen(destination))
	common.Must1(io.ReadFull(c.secureRNG, header[:c.keySaltLength]))
	err := M.SocksaddrSerializer.WriteAddrPort(buf.With(header[c.keySaltLength:]), destination)
	if err != nil {
		return err
	}
	err = c.EncodePacket(buffer)
	if err != nil {
		return err
	}
	return common.Error(c.Write(buffer.Bytes()))
}

func (c *clientPacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	n, err := c.Read(buffer.FreeBytes())
	if err != nil {
		return M.Socksaddr{}, err
	}
	buffer.Truncate(n)
	err = c.DecodePacket(buffer)
	if err != nil {
		return M.Socksaddr{}, err
	}
	return M.SocksaddrSerializer.ReadAddrPort(buffer)
}

func (c *clientPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, err = c.Read(p)
	if err != nil {
		return
	}
	b := buf.With(p[:n])
	err = c.DecodePacket(b)
	if err != nil {
		return
	}
	destination, err := M.SocksaddrSerializer.ReadAddrPort(b)
	if err != nil {
		return
	}
	addr = destination.UDPAddr()
	n = copy(p, b.Bytes())
	return
}

func (c *clientPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	_buffer := buf.StackNew()
	defer runtime.KeepAlive(_buffer)
	buffer := common.Dup(_buffer)
	err = M.SocksaddrSerializer.WriteAddrPort(buffer, M.SocksaddrFromNet(addr))
	if err != nil {
		return
	}
	_, err = buffer.Write(p)
	if err != nil {
		return
	}
	err = c.EncodePacket(buffer)
	if err != nil {
		return
	}
	_, err = c.Write(buffer.Bytes())
	if err != nil {
		return
	}
	return len(p), nil
}

func (c *clientPacketConn) UpstreamReader() io.Reader {
	return c.Conn
}

func (c *clientPacketConn) ReaderReplaceable() bool {
	return false
}

func (c *clientPacketConn) UpstreamWriter() io.Writer {
	return c.Conn
}

func (c *clientPacketConn) WriterReplaceable() bool {
	return false
}
