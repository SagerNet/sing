package shadowaead_2022

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"io"
	"math"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/replay"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/protocol/shadowsocks"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead"
	"github.com/sagernet/sing/protocol/socks"
	"golang.org/x/crypto/chacha20poly1305"
	wgReplay "golang.zx2c4.com/wireguard/replay"
	"lukechampine.com/blake3"
)

const (
	HeaderTypeClient      = 0
	HeaderTypeServer      = 1
	MaxPaddingLength      = 900
	KeySaltSize           = 32
	PacketNonceSize       = 24
	MinRequestHeaderSize  = 1 + 8
	MinResponseHeaderSize = MinRequestHeaderSize + KeySaltSize
	MaxPacketSize         = 64 * 1024
)

var (
	ErrBadHeaderType      = E.New("bad header type")
	ErrBadTimestamp       = E.New("bad timestamp")
	ErrBadRequestSalt     = E.New("bad request salt")
	ErrBadClientSessionId = E.New("bad client session id")
	ErrPacketIdNotUnique  = E.New("packet id not unique")
)

var List = []string{
	"2022-blake3-aes-128-gcm",
	"2022-blake3-aes-256-gcm",
	"2022-blake3-chacha20-poly1305",
}

func New(method string, psk []byte, secureRNG io.Reader) (shadowsocks.Method, error) {
	m := &Method{
		name:         method,
		key:          psk,
		secureRNG:    secureRNG,
		replayFilter: replay.NewCuckoo(30),
	}

	if len(psk) != KeySaltSize {
		return nil, shadowaead.ErrBadKey
	}

	switch method {
	case "2022-blake3-aes-128-gcm":
		m.keyLength = 16
		m.constructor = newAESGCM
		m.udpBlockConstructor = newAES
	case "2022-blake3-aes-256-gcm":
		m.keyLength = 32
		m.constructor = newAESGCM
		m.udpBlockConstructor = newAES
	case "2022-blake3-chacha20-poly1305":
		m.keyLength = 32
		m.constructor = func(key []byte) cipher.AEAD {
			cipher, err := chacha20poly1305.New(key)
			common.Must(err)
			return cipher
		}
		m.udpConstructor = func(key []byte) cipher.AEAD {
			cipher, err := chacha20poly1305.NewX(key)
			common.Must(err)
			return cipher
		}
	}
	return m, nil
}

func Blake3DeriveKey(secret, salt, outKey []byte) {
	sessionKey := make([]byte, len(secret)+len(salt))
	copy(sessionKey, secret)
	copy(sessionKey[len(secret):], salt)
	blake3.DeriveKey(outKey, "shadowsocks 2022 session subkey", sessionKey)
}

func newAES(key []byte) cipher.Block {
	block, err := aes.NewCipher(key)
	common.Must(err)
	return block
}

func newAESGCM(key []byte) cipher.AEAD {
	block, err := aes.NewCipher(key)
	common.Must(err)
	aead, err := cipher.NewGCM(block)
	common.Must(err)
	return aead
}

type Method struct {
	name                string
	keyLength           int
	constructor         func(key []byte) cipher.AEAD
	udpBlockConstructor func(key []byte) cipher.Block
	udpConstructor      func(key []byte) cipher.AEAD
	key                 []byte
	secureRNG           io.Reader
	replayFilter        replay.Filter
}

func (m *Method) Name() string {
	return m.name
}

func (m *Method) KeyLength() int {
	return m.keyLength
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
	return &clientPacketConn{conn, m, newUDPSession()}
}

func (m *Method) EncodePacket(buffer *buf.Buffer) error {
	if m.udpConstructor == nil {
		// aes
		packetHeader := buffer.To(aes.BlockSize)
		subKey := make([]byte, m.keyLength)
		Blake3DeriveKey(m.key, packetHeader[:8], subKey)

		cipher := m.constructor(subKey)
		cipher.Seal(buffer.Index(aes.BlockSize), packetHeader[4:16], buffer.From(aes.BlockSize), nil)
		buffer.Extend(cipher.Overhead())
		m.udpBlockConstructor(m.key).Encrypt(packetHeader, packetHeader)
	} else {
		// xchacha
		cipher := m.udpConstructor(m.key)
		cipher.Seal(buffer.Index(PacketNonceSize), buffer.To(PacketNonceSize), buffer.From(PacketNonceSize), nil)
		buffer.Extend(cipher.Overhead())
	}
	return nil
}

func (m *Method) DecodePacket(buffer *buf.Buffer) error {
	if m.udpBlockConstructor != nil {
		if buffer.Len() <= aes.BlockSize {
			return E.New("insufficient data: ", buffer.Len())
		}
		packetHeader := buffer.To(aes.BlockSize)
		m.udpBlockConstructor(m.key).Decrypt(packetHeader, packetHeader)
		subKey := make([]byte, m.keyLength)
		Blake3DeriveKey(m.key, packetHeader[:8], subKey)
		_, err := m.constructor(subKey).Open(buffer.Index(aes.BlockSize), packetHeader[4:16], buffer.From(aes.BlockSize), nil)
		if err != nil {
			return err
		}
	} else {
		_, err := m.udpConstructor(m.key).Open(buffer.Index(PacketNonceSize), buffer.To(PacketNonceSize), buffer.From(PacketNonceSize), nil)
		if err != nil {
			return err
		}
		buffer.Advance(PacketNonceSize)
	}
	return nil
}

type clientConn struct {
	net.Conn

	method      *Method
	destination *M.AddrPort

	request  sync.Mutex
	response sync.Mutex

	requestSalt []byte

	reader io.Reader
	writer io.Writer
}

func (c *clientConn) writeRequest(payload []byte) error {
	request := buf.New()
	defer request.Release()

	salt := make([]byte, KeySaltSize)
	common.Must1(io.ReadFull(c.method.secureRNG, salt))
	c.method.replayFilter.Check(salt)
	common.Must1(request.Write(salt))

	subKey := make([]byte, c.method.keyLength)
	Blake3DeriveKey(c.method.key, salt, subKey)

	var writer io.Writer = c.Conn
	writer = &buf.BufferedWriter{
		Writer: writer,
		Buffer: request,
	}
	writer = shadowaead.NewWriter(
		writer,
		c.method.constructor(subKey),
		MaxPacketSize,
	)

	header := buf.New()
	defer header.Release()

	writer = &buf.BufferedWriter{
		Writer: writer,
		Buffer: header,
	}

	common.Must(rw.WriteByte(writer, HeaderTypeClient))
	common.Must(binary.Write(writer, binary.BigEndian, uint64(time.Now().Unix())))

	err := socks.AddressSerializer.WriteAddrPort(writer, c.destination)
	if err != nil {
		return E.Cause(err, "write destination")
	}

	if len(payload) > 0 {
		err = binary.Write(writer, binary.BigEndian, uint16(0))
		if err != nil {
			return E.Cause(err, "write padding length")
		}
		_, err = writer.Write(payload)
		if err != nil {
			return E.Cause(err, "write payload")
		}
	} else {
		pLen := rand.Intn(MaxPaddingLength)
		err = binary.Write(writer, binary.BigEndian, uint16(pLen))
		if err != nil {
			return E.Cause(err, "write padding length")
		}
		_, err = io.CopyN(writer, c.method.secureRNG, int64(pLen))
		if err != nil {
			return E.Cause(err, "write padding")
		}
	}

	err = common.FlushVar(&writer)
	if err != nil {
		return E.Cause(err, "client handshake")
	}
	c.requestSalt = salt
	c.writer = writer
	return nil
}

func (c *clientConn) readResponse() error {
	if c.reader != nil {
		return nil
	}

	c.response.Lock()
	defer c.response.Unlock()

	if c.reader != nil {
		return nil
	}

	salt := make([]byte, KeySaltSize)
	_, err := io.ReadFull(c.Conn, salt)
	if err != nil {
		return err
	}

	if !c.method.replayFilter.Check(salt) {
		return E.New("salt is not unique")
	}

	subKey := make([]byte, c.method.keyLength)
	Blake3DeriveKey(c.method.key, salt, subKey)

	reader := shadowaead.NewReader(
		c.Conn,
		c.method.constructor(subKey),
		MaxPacketSize,
	)

	headerType, err := rw.ReadByte(reader)
	if err != nil {
		return err
	}
	if headerType != HeaderTypeServer {
		return ErrBadHeaderType
	}

	var epoch uint64
	err = binary.Read(reader, binary.BigEndian, &epoch)
	if err != nil {
		return err
	}
	if math.Abs(float64(time.Now().Unix()-int64(epoch))) > 30 {
		return ErrBadTimestamp
	}

	requestSalt := make([]byte, KeySaltSize)
	_, err = io.ReadFull(reader, requestSalt)
	if err != nil {
		return err
	}

	if bytes.Compare(requestSalt, c.requestSalt) > 0 {
		return ErrBadRequestSalt
	}

	c.reader = reader
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
	return c.reader.(io.WriterTo).WriteTo(w)
}

func (c *clientConn) Write(p []byte) (n int, err error) {
	if c.writer != nil {
		return c.writer.Write(p)
	}

	c.request.Lock()

	if c.writer != nil {
		c.request.Unlock()
		return c.writer.Write(p)
	}

	defer c.request.Unlock()

	err = c.writeRequest(p)
	if err == nil {
		n = len(p)
	}
	return
}

func (c *clientConn) ReadFrom(r io.Reader) (n int64, err error) {
	if c.writer == nil {
		panic("missing client handshake")
	}

	return c.writer.(io.ReaderFrom).ReadFrom(r)
}

type clientPacketConn struct {
	net.Conn
	method  *Method
	session *udpSession
}

func (c *clientPacketConn) WritePacket(buffer *buf.Buffer, destination *M.AddrPort) error {
	defer buffer.Release()
	header := buf.New()
	if c.method.udpConstructor != nil {
		common.Must1(header.ReadFullFrom(c.method.secureRNG, PacketNonceSize))
	}
	common.Must(
		binary.Write(header, binary.BigEndian, c.session.sessionId),
		binary.Write(header, binary.BigEndian, c.session.nextPacketId()),
		header.WriteByte(HeaderTypeClient),
		binary.Write(header, binary.BigEndian, uint64(time.Now().Unix())),
		binary.Write(header, binary.BigEndian, uint16(0)), // padding length
	)
	c.session.filter.ValidateCounter(c.session.packetId, math.MaxUint64)
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

	var sessionId uint64
	err = binary.Read(buffer, binary.BigEndian, &sessionId)
	if err != nil {
		return nil, err
	}

	var isLastSessionId bool
	if c.session.remoteSessionId == 0 {
		c.session.remoteSessionId = sessionId
	} else if sessionId != c.session.remoteSessionId {
		if sessionId == c.session.lastRemoteSessionId {
			isLastSessionId = true
		} else {
			c.session.lastRemoteSessionId = c.session.remoteSessionId
			c.session.remoteSessionId = sessionId
			c.session.lastFilter = c.session.filter
			c.session.filter = new(wgReplay.Filter)
		}
	}

	var packetId uint64
	err = binary.Read(buffer, binary.BigEndian, &packetId)
	if err != nil {
		return nil, err
	}
	if !isLastSessionId {
		if !c.session.filter.ValidateCounter(packetId, math.MaxUint64) {
			return nil, ErrPacketIdNotUnique
		}
	} else {
		if !c.session.lastFilter.ValidateCounter(packetId, math.MaxUint64) {
			return nil, ErrPacketIdNotUnique
		}
	}

	headerType, err := buffer.ReadBytes(1)
	if err != nil {
		return nil, err
	}
	if headerType[0] != HeaderTypeServer {
		return nil, ErrBadHeaderType
	}

	var epoch uint64
	err = binary.Read(buffer, binary.BigEndian, &epoch)
	if err != nil {
		return nil, err
	}
	if math.Abs(float64(uint64(time.Now().Unix())-epoch)) > 30 {
		return nil, ErrBadTimestamp
	}

	var clientSessionId uint64
	err = binary.Read(buffer, binary.BigEndian, &clientSessionId)
	if err != nil {
		return nil, err
	}

	if clientSessionId != c.session.sessionId {
		return nil, ErrBadClientSessionId
	}

	var paddingLength uint16
	err = binary.Read(buffer, binary.BigEndian, &paddingLength)
	if err != nil {
		return nil, E.Cause(err, "read padding length")
	}
	buffer.Advance(int(paddingLength))
	return socks.AddressSerializer.ReadAddrPort(buffer)
}

type udpSession struct {
	headerType          byte
	sessionId           uint64
	packetId            uint64
	remoteSessionId     uint64
	lastRemoteSessionId uint64
	filter              *wgReplay.Filter
	lastFilter          *wgReplay.Filter
}

func (s *udpSession) nextPacketId() uint64 {
	return atomic.AddUint64(&s.packetId, 1)
}

func newUDPSession() *udpSession {
	return &udpSession{
		sessionId: rand.Uint64(),
		filter:    new(wgReplay.Filter),
	}
}
