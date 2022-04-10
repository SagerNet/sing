package shadowaead

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"github.com/sagernet/sing/common/replay"
	"io"
	"net"
	"sync"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
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

func New(method string, secureRNG io.Reader) shadowsocks.Method {
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
	return m
}

func NewSession(key []byte, replayFilter bool) shadowsocks.Session {
	var filter replay.Filter
	if replayFilter {
		filter = replay.NewBloomRing()
	}
	return &session{key, filter}
}

func Kdf(key, iv []byte, keyLength int) []byte {
	subKey := make([]byte, keyLength)
	kdf := hkdf.New(sha1.New, key, iv, []byte("ss-subkey"))
	common.Must1(io.ReadFull(kdf, subKey))
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
	secureRNG     io.Reader
}

func (m *Method) Name() string {
	return m.name
}

func (m *Method) KeyLength() int {
	return m.keySaltLength
}

func (m *Method) DialConn(account shadowsocks.Session, conn net.Conn, destination *M.AddrPort) (net.Conn, error) {
	shadowsocksConn := &aeadConn{
		Conn:         conn,
		method:       m,
		key:          account.Key(),
		replayFilter: account.ReplayFilter(),
		destination:  destination,
	}
	return shadowsocksConn, shadowsocksConn.clientHandshake()
}

func (m *Method) DialEarlyConn(account shadowsocks.Session, conn net.Conn, destination *M.AddrPort) net.Conn {
	return &aeadConn{
		Conn:         conn,
		method:       m,
		key:          account.Key(),
		replayFilter: account.ReplayFilter(),
		destination:  destination,
	}
}

func (m *Method) DialPacketConn(account shadowsocks.Session, conn net.Conn) socks.PacketConn {
	return &aeadPacketConn{conn, account.Key(), m}
}

func (m *Method) EncodePacket(key []byte, buffer *buf.Buffer) error {
	cipher := m.constructor(Kdf(key, buffer.To(m.keySaltLength), m.keySaltLength))
	cipher.Seal(buffer.From(m.keySaltLength)[:0], rw.ZeroBytes[:cipher.NonceSize()], buffer.From(m.keySaltLength), nil)
	buffer.Extend(cipher.Overhead())
	return nil
}

func (m *Method) DecodePacket(key []byte, buffer *buf.Buffer) error {
	if buffer.Len() < m.keySaltLength {
		return E.New("bad packet")
	}
	aead := m.constructor(Kdf(key, buffer.To(m.keySaltLength), m.keySaltLength))
	packet, err := aead.Open(buffer.Index(m.keySaltLength), rw.ZeroBytes[:aead.NonceSize()], buffer.From(m.keySaltLength), nil)
	if err != nil {
		return err
	}
	buffer.Advance(m.keySaltLength)
	buffer.Truncate(len(packet))
	return nil
}

type session struct {
	key          []byte
	replayFilter replay.Filter
}

func (a *session) Key() []byte {
	return a.key
}

func (a *session) ReplayFilter() replay.Filter {
	return a.replayFilter
}

type aeadConn struct {
	net.Conn

	method      *Method
	key         []byte
	destination *M.AddrPort

	access       sync.Mutex
	reader       io.Reader
	writer       io.Writer
	replayFilter replay.Filter
}

func (c *aeadConn) clientHandshake() error {
	header := buf.New()
	defer header.Release()

	common.Must1(header.ReadFullFrom(c.method.secureRNG, c.method.keySaltLength))
	if c.replayFilter != nil {
		c.replayFilter.Check(header.Bytes())
	}

	c.writer = NewAEADWriter(
		&buf.BufferedWriter{
			Writer: c.Conn,
			Buffer: header,
		},
		c.method.constructor(Kdf(c.key, header.Bytes(), c.method.keySaltLength)),
	)

	err := socks.AddressSerializer.WriteAddrPort(c.writer, c.destination)
	if err != nil {
		return err
	}

	return common.FlushVar(&c.writer)
}

func (c *aeadConn) serverHandshake() error {
	if c.reader == nil {
		salt := make([]byte, c.method.keySaltLength)
		_, err := io.ReadFull(c.Conn, salt)
		if err != nil {
			return err
		}
		if c.replayFilter != nil {
			if !c.replayFilter.Check(salt) {
				return E.New("salt is not unique")
			}
		}
		c.reader = NewReader(c.Conn, c.method.constructor(Kdf(c.key, salt, c.method.keySaltLength)))
	}
	return nil
}

func (c *aeadConn) Read(p []byte) (n int, err error) {
	if err = c.serverHandshake(); err != nil {
		return
	}
	return c.reader.Read(p)
}

func (c *aeadConn) WriteTo(w io.Writer) (n int64, err error) {
	if err = c.serverHandshake(); err != nil {
		return
	}
	return c.reader.(io.WriterTo).WriteTo(w)
}

func (c *aeadConn) Write(p []byte) (n int, err error) {
	if c.writer != nil {
		goto direct
	}

	c.access.Lock()
	defer c.access.Unlock()

	if c.writer != nil {
		goto direct
	}

	// client handshake

	{
		header := buf.New()
		defer header.Release()

		request := buf.New()
		defer request.Release()

		common.Must1(header.ReadFullFrom(c.method.secureRNG, c.method.keySaltLength))
		if c.replayFilter != nil {
			c.replayFilter.Check(header.Bytes())
		}

		var writer io.Writer = c.Conn
		writer = &buf.BufferedWriter{
			Writer: writer,
			Buffer: header,
		}
		writer = NewAEADWriter(writer, c.method.constructor(Kdf(c.key, header.Bytes(), c.method.keySaltLength)))
		writer = &buf.BufferedWriter{
			Writer: writer,
			Buffer: request,
		}

		err = socks.AddressSerializer.WriteAddrPort(writer, c.destination)
		if err != nil {
			return
		}

		if len(p) > 0 {
			_, err = writer.Write(p)
			if err != nil {
				return
			}
		}

		err = common.FlushVar(&writer)
		if err != nil {
			return
		}

		c.writer = writer
		return len(p), nil
	}

direct:
	return c.writer.Write(p)
}

func (c *aeadConn) ReadFrom(r io.Reader) (n int64, err error) {
	if c.writer == nil {
		panic("missing client handshake")
	}
	return c.writer.(io.ReaderFrom).ReadFrom(r)
}

type aeadPacketConn struct {
	net.Conn
	key    []byte
	method *Method
}

func (c *aeadPacketConn) WritePacket(buffer *buf.Buffer, destination *M.AddrPort) error {
	defer buffer.Release()
	header := buf.New()
	common.Must1(header.ReadFullFrom(c.method.secureRNG, c.method.keySaltLength))
	err := socks.AddressSerializer.WriteAddrPort(header, destination)
	if err != nil {
		return err
	}
	buffer = buffer.WriteBufferAtFirst(header)
	err = c.method.EncodePacket(c.key, buffer)
	if err != nil {
		return err
	}
	return common.Error(c.Write(buffer.Bytes()))
}

func (c *aeadPacketConn) ReadPacket(buffer *buf.Buffer) (*M.AddrPort, error) {
	n, err := c.Read(buffer.FreeBytes())
	if err != nil {
		return nil, err
	}
	buffer.Truncate(n)
	err = c.method.DecodePacket(c.key, buffer)
	if err != nil {
		return nil, err
	}
	return socks.AddressSerializer.ReadAddrPort(buffer)
}
