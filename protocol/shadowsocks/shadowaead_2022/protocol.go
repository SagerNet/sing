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
	ErrBadHeaderType         = E.New("bad header type")
	ErrBadTimestamp          = E.New("bad timestamp")
	ErrBadRequestSalt        = E.New("bad request salt")
	ErrBadClientSessionId    = E.New("bad client session id")
	ErrPacketIdNotUnique     = E.New("packet id not unique")
	ErrTooManyServerSessions = E.New("server session changed more than once during the last minute")
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
		replayFilter: replay.NewCuckoo(60),
	}

	if len(psk) != KeySaltSize {
		return nil, shadowaead.ErrBadKey
	}

	switch method {
	case "2022-blake3-aes-128-gcm":
		m.keyLength = 16
		m.constructor = newAESGCM
		m.udpBlockCipher = newAES(psk)
	case "2022-blake3-aes-256-gcm":
		m.keyLength = 32
		m.constructor = newAESGCM
		m.udpBlockCipher = newAES(psk)
	case "2022-blake3-chacha20-poly1305":
		m.keyLength = 32
		m.constructor = newChacha20Poly1305
		m.udpCipher = newXChacha20Poly1305(psk)
	}
	return m, nil
}

func Blake3DeriveKey(secret, salt []byte, keyLength int) []byte {
	sessionKey := make([]byte, len(secret)+len(salt))
	copy(sessionKey, secret)
	copy(sessionKey[len(secret):], salt)
	outKey := make([]byte, keyLength)
	blake3.DeriveKey(outKey, "shadowsocks 2022 session subkey", sessionKey)
	return outKey
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

func newChacha20Poly1305(key []byte) cipher.AEAD {
	cipher, err := chacha20poly1305.New(key)
	common.Must(err)
	return cipher
}

func newXChacha20Poly1305(key []byte) cipher.AEAD {
	cipher, err := chacha20poly1305.NewX(key)
	common.Must(err)
	return cipher
}

type Method struct {
	name           string
	keyLength      int
	constructor    func(key []byte) cipher.AEAD
	udpCipher      cipher.AEAD
	udpBlockCipher cipher.Block
	key            []byte
	secureRNG      io.Reader
	replayFilter   replay.Filter
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
	return &clientPacketConn{conn, m, m.newUDPSession()}
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
	common.Must1(request.Write(salt))

	var writer io.Writer = c.Conn
	writer = &buf.BufferedWriter{
		Writer: writer,
		Buffer: request,
	}
	writer = shadowaead.NewWriter(
		writer,
		c.method.constructor(Blake3DeriveKey(c.method.key, salt, c.method.keyLength)),
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
		pLen := rand.Intn(MaxPaddingLength + 1)
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

	reader := shadowaead.NewReader(
		c.Conn,
		c.method.constructor(Blake3DeriveKey(c.method.key, salt, c.method.keyLength)),
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
	if c.method.udpCipher != nil {
		common.Must1(header.ReadFullFrom(c.method.secureRNG, PacketNonceSize))
	}
	common.Must(
		binary.Write(header, binary.BigEndian, c.session.sessionId),
		binary.Write(header, binary.BigEndian, c.session.nextPacketId()),
		header.WriteByte(HeaderTypeClient),
		binary.Write(header, binary.BigEndian, uint64(time.Now().Unix())),
		binary.Write(header, binary.BigEndian, uint16(0)), // padding length
	)
	err := socks.AddressSerializer.WriteAddrPort(header, destination)
	if err != nil {
		return err
	}
	buffer = buffer.WriteBufferAtFirst(header)
	if c.method.udpCipher != nil {
		c.method.udpCipher.Seal(buffer.Index(PacketNonceSize), buffer.To(PacketNonceSize), buffer.From(PacketNonceSize), nil)
		buffer.Extend(c.method.udpCipher.Overhead())
	} else {
		packetHeader := buffer.To(aes.BlockSize)
		c.session.cipher.Seal(buffer.Index(aes.BlockSize), packetHeader[4:16], buffer.From(aes.BlockSize), nil)
		buffer.Extend(c.session.cipher.Overhead())
		c.method.udpBlockCipher.Encrypt(packetHeader, packetHeader)
	}
	return common.Error(c.Write(buffer.Bytes()))
}

func (c *clientPacketConn) ReadPacket(buffer *buf.Buffer) (*M.AddrPort, error) {
	n, err := c.Read(buffer.FreeBytes())
	if err != nil {
		return nil, err
	}
	buffer.Truncate(n)

	var packetHeader []byte
	if c.method.udpCipher != nil {
		_, err = c.method.udpCipher.Open(buffer.Index(PacketNonceSize), buffer.To(PacketNonceSize), buffer.From(PacketNonceSize), nil)
		if err != nil {
			return nil, E.Cause(err, "decrypt packet")
		}
		buffer.Advance(PacketNonceSize)
	} else {
		packetHeader = buffer.To(aes.BlockSize)
		c.method.udpBlockCipher.Decrypt(packetHeader, packetHeader)
	}

	var sessionId, packetId uint64
	err = binary.Read(buffer, binary.BigEndian, &sessionId)
	if err != nil {
		return nil, err
	}
	err = binary.Read(buffer, binary.BigEndian, &packetId)
	if err != nil {
		return nil, err
	}

	var remoteCipher cipher.AEAD
	if packetHeader != nil {
		if sessionId == c.session.remoteSessionId {
			remoteCipher = c.session.remoteCipher
		} else if sessionId == c.session.lastRemoteSessionId {
			remoteCipher = c.session.lastRemoteCipher
		} else {
			remoteCipher = c.method.constructor(Blake3DeriveKey(c.method.key, packetHeader[:8], c.method.keyLength))
		}
		_, err = remoteCipher.Open(buffer.Index(0), packetHeader[4:16], buffer.Bytes(), nil)
		if err != nil {
			return nil, E.Cause(err, "decrypt packet")
		}
	}

	var headerType byte
	headerType, err = buffer.ReadByte()
	if err != nil {
		return nil, err
	}
	if headerType != HeaderTypeServer {
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

	if sessionId == c.session.remoteSessionId {
		if !c.session.filter.ValidateCounter(packetId, math.MaxUint64) {
			return nil, ErrPacketIdNotUnique
		}
		c.session.remoteSeen = time.Now().Unix()
	} else if sessionId == c.session.lastRemoteSessionId {
		if !c.session.lastFilter.ValidateCounter(packetId, math.MaxUint64) {
			return nil, ErrPacketIdNotUnique
		}
		remoteCipher = c.session.lastRemoteCipher
		c.session.lastRemoteSeen = time.Now().Unix()
	} else {
		if c.session.remoteSessionId != 0 {
			if time.Now().Unix()-c.session.lastRemoteSeen < 60 {
				return nil, ErrTooManyServerSessions
			} else {
				c.session.lastRemoteSessionId = c.session.remoteSessionId
				c.session.lastFilter = c.session.filter
				c.session.lastRemoteSeen = c.session.remoteSeen
				c.session.lastRemoteCipher = c.session.remoteCipher
				c.session.filter = new(wgReplay.Filter)
			}
		}
		c.session.remoteSessionId = sessionId
		c.session.remoteCipher = remoteCipher
		c.session.remoteSeen = time.Now().Unix()
		c.session.filter.ValidateCounter(packetId, math.MaxUint64)
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
	remoteSeen          int64
	lastRemoteSeen      int64
	cipher              cipher.AEAD
	remoteCipher        cipher.AEAD
	lastRemoteCipher    cipher.AEAD
	filter              *wgReplay.Filter
	lastFilter          *wgReplay.Filter
}

func (s *udpSession) nextPacketId() uint64 {
	return atomic.AddUint64(&s.packetId, 1)
}

func (m *Method) newUDPSession() *udpSession {
	session := &udpSession{
		sessionId: rand.Uint64(),
		filter:    new(wgReplay.Filter),
	}
	if m.udpCipher == nil {
		sessionId := make([]byte, 8)
		binary.BigEndian.PutUint64(sessionId, session.sessionId)
		session.cipher = m.constructor(Blake3DeriveKey(m.key, sessionId, m.keyLength))
	}
	return session
}
