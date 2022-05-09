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
	"runtime"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/replay"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/protocol/shadowsocks"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead"
	"github.com/sagernet/sing/protocol/socks5"
	"golang.org/x/crypto/chacha20poly1305"
	wgReplay "golang.zx2c4.com/wireguard/replay"
	"lukechampine.com/blake3"
)

const (
	HeaderTypeClient = 0
	HeaderTypeServer = 1
	MaxPaddingLength = 900
	KeySaltSize      = 32
	PacketNonceSize  = 24
	MaxPacketSize    = 65535
)

const (
	// crypto/cipher.gcmStandardNonceSize
	// golang.org/x/crypto/chacha20poly1305.NonceSize
	nonceSize = 12

	// Overhead
	// crypto/cipher.gcmTagSize
	// golang.org/x/crypto/chacha20poly1305.Overhead
	overhead = 16
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

func New(method string, pskList [][KeySaltSize]byte, secureRNG io.Reader) (shadowsocks.Method, error) {
	m := &Method{
		name:         method,
		psk:          pskList[len(pskList)-1],
		pskList:      pskList,
		secureRNG:    secureRNG,
		replayFilter: replay.NewCuckoo(60),
	}

	if len(pskList) > 1 {
		pskHash := make([]byte, (len(pskList)-1)*aes.BlockSize)
		for i, psk := range pskList {
			if i == 0 {
				continue
			}
			hash := blake3.Sum512(psk[:])
			copy(pskHash[aes.BlockSize*(i-1):aes.BlockSize*i], hash[:aes.BlockSize])
		}
		m.pskHash = pskHash
	}

	switch method {
	case "2022-blake3-aes-128-gcm":
		m.keyLength = 16
		m.constructor = newAESGCM
		m.blockConstructor = newAES
		m.udpBlockCipher = newAES(pskList[0][:])
	case "2022-blake3-aes-256-gcm":
		m.keyLength = 32
		m.constructor = newAESGCM
		m.blockConstructor = newAES
		m.udpBlockCipher = newAES(pskList[0][:])
	case "2022-blake3-chacha20-poly1305":
		m.keyLength = 32
		m.constructor = newChacha20Poly1305
		m.udpCipher = newXChacha20Poly1305(m.psk[:])
	}
	return m, nil
}

func Blake3DeriveKey(psk []byte, salt []byte, keyLength int) []byte {
	sessionKey := buf.Make(len(psk) + len(salt))
	copy(sessionKey, psk[:])
	copy(sessionKey[len(psk):], salt)
	outKey := buf.Make(keyLength)
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
	name             string
	keyLength        int
	constructor      func(key []byte) cipher.AEAD
	blockConstructor func(key []byte) cipher.Block
	udpCipher        cipher.AEAD
	udpBlockCipher   cipher.Block
	psk              [KeySaltSize]byte
	pskList          [][KeySaltSize]byte
	pskHash          []byte
	secureRNG        io.Reader
	replayFilter     replay.Filter
}

func (m *Method) Name() string {
	return m.name
}

func (m *Method) KeyLength() int {
	return m.keyLength
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
	return &clientPacketConn{conn, m, m.newUDPSession()}
}

type clientConn struct {
	net.Conn

	method      *Method
	destination M.Socksaddr

	requestSalt []byte

	reader *shadowaead.Reader
	writer *shadowaead.Writer
}

func (m *Method) writeExtendedIdentityHeaders(request *buf.Buffer, salt []byte) {
	pskLen := len(m.pskList)
	if pskLen < 2 {
		return
	}
	for i, psk := range m.pskList {
		keyMaterial := make([]byte, 2*KeySaltSize)
		copy(keyMaterial, psk[:])
		copy(keyMaterial[KeySaltSize:], salt)
		_identitySubkey := buf.Make(m.keyLength)
		identitySubkey := common.Dup(_identitySubkey)
		blake3.DeriveKey(identitySubkey, "shadowsocks 2022 identity subkey", keyMaterial)

		pskHash := m.pskHash[aes.BlockSize*i : aes.BlockSize*(i+1)]

		header := request.Extend(16)
		m.blockConstructor(identitySubkey).Encrypt(header, pskHash)
		runtime.KeepAlive(_identitySubkey)
		if i == pskLen-2 {
			break
		}
	}
}

func (c *clientConn) writeRequest(payload []byte) error {
	salt := make([]byte, KeySaltSize)
	common.Must1(io.ReadFull(c.method.secureRNG, salt))

	key := Blake3DeriveKey(c.method.psk[:], salt, c.method.keyLength)
	writer := shadowaead.NewWriter(
		c.Conn,
		c.method.constructor(common.Dup(key)),
		MaxPacketSize,
	)
	runtime.KeepAlive(key)

	header := writer.Buffer()
	header.Write(salt)
	c.method.writeExtendedIdentityHeaders(header, salt)

	bufferedWriter := writer.BufferedWriter(header.Len())

	common.Must(rw.WriteByte(bufferedWriter, HeaderTypeClient))
	common.Must(binary.Write(bufferedWriter, binary.BigEndian, uint64(time.Now().Unix())))

	err := socks5.AddressSerializer.WriteAddrPort(bufferedWriter, c.destination)
	if err != nil {
		return E.Cause(err, "write destination")
	}

	if len(payload) > 0 {
		err = binary.Write(bufferedWriter, binary.BigEndian, uint16(0))
		if err != nil {
			return E.Cause(err, "write padding length")
		}
		_, err = bufferedWriter.Write(payload)
		if err != nil {
			return E.Cause(err, "write payload")
		}
	} else {
		pLen := rand.Intn(MaxPaddingLength + 1)
		err = binary.Write(bufferedWriter, binary.BigEndian, uint16(pLen))
		if err != nil {
			return E.Cause(err, "write padding length")
		}
		_, err = io.CopyN(bufferedWriter, c.method.secureRNG, int64(pLen))
		if err != nil {
			return E.Cause(err, "write padding")
		}
	}

	err = bufferedWriter.Flush()
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

	_salt := make([]byte, KeySaltSize)
	salt := common.Dup(_salt)
	_, err := io.ReadFull(c.Conn, salt)
	if err != nil {
		return err
	}

	if !c.method.replayFilter.Check(salt) {
		return E.New("salt not unique")
	}

	key := Blake3DeriveKey(c.method.psk[:], salt, c.method.keyLength)
	runtime.KeepAlive(_salt)
	reader := shadowaead.NewReader(
		c.Conn,
		c.method.constructor(common.Dup(key)),
		MaxPacketSize,
	)
	runtime.KeepAlive(key)

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

	diff := int(math.Abs(float64(time.Now().Unix() - int64(epoch))))
	if diff > 30 {
		return ErrBadTimestamp
	}

	_requestSalt := make([]byte, KeySaltSize)
	requestSalt := common.Dup(_requestSalt)
	_, err = io.ReadFull(reader, requestSalt)
	if err != nil {
		return err
	}

	if bytes.Compare(requestSalt, c.requestSalt) > 0 {
		return ErrBadRequestSalt
	}
	runtime.KeepAlive(_requestSalt)

	c.requestSalt = nil
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
	return c.reader.WriteTo(w)
}

func (c *clientConn) Write(p []byte) (n int, err error) {
	if c.writer == nil {
		err = c.writeRequest(p)
		if err == nil {
			n = len(p)
		}
		return
	}
	return c.writer.Write(p)
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
	net.Conn
	method  *Method
	session *udpSession
}

func (c *clientPacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	var hdrLen int
	if c.method.udpCipher != nil {
		hdrLen = PacketNonceSize
	}
	hdrLen += 16 // packet header
	pskLen := len(c.method.pskList)
	if c.method.udpCipher == nil && pskLen > 1 {
		hdrLen += (pskLen - 1) * aes.BlockSize
	}
	hdrLen += 1 // header type
	hdrLen += 8 // timestamp
	hdrLen += 2 // padding length
	hdrLen += socks5.AddressSerializer.AddrPortLen(destination)
	header := buf.With(buffer.ExtendHeader(hdrLen))

	var dataIndex int
	if c.method.udpCipher != nil {
		common.Must1(header.ReadFullFrom(c.method.secureRNG, PacketNonceSize))
		if pskLen > 1 {
			panic("unsupported chacha extended header")
		}
		dataIndex = buffer.Len()
	} else {
		dataIndex = aes.BlockSize
	}

	common.Must(
		binary.Write(header, binary.BigEndian, c.session.sessionId),
		binary.Write(header, binary.BigEndian, c.session.nextPacketId()),
	)

	if c.method.udpCipher == nil && pskLen > 1 {
		for i, psk := range c.method.pskList {
			dataIndex += aes.BlockSize
			pskHash := c.method.pskHash[aes.BlockSize*i : aes.BlockSize*(i+1)]

			identityHeader := header.Extend(aes.BlockSize)
			for textI := 0; textI < aes.BlockSize; textI++ {
				identityHeader[textI] = pskHash[textI] ^ header.Byte(textI)
			}
			c.method.blockConstructor(psk[:]).Encrypt(identityHeader, identityHeader)

			if i == pskLen-2 {
				break
			}
		}
	}
	common.Must(
		header.WriteByte(HeaderTypeClient),
		binary.Write(header, binary.BigEndian, uint64(time.Now().Unix())),
		binary.Write(header, binary.BigEndian, uint16(0)), // padding length
	)
	err := socks5.AddressSerializer.WriteAddrPort(header, destination)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	if c.method.udpCipher != nil {
		c.method.udpCipher.Seal(buffer.Index(dataIndex), buffer.To(dataIndex), buffer.From(dataIndex), nil)
		buffer.Extend(c.method.udpCipher.Overhead())
	} else {
		packetHeader := buffer.To(aes.BlockSize)
		c.session.cipher.Seal(buffer.Index(dataIndex), packetHeader[4:16], buffer.From(dataIndex), nil)
		buffer.Extend(c.session.cipher.Overhead())
		c.method.udpBlockCipher.Encrypt(packetHeader, packetHeader)
	}
	return common.Error(c.Write(buffer.Bytes()))
}

func (c *clientPacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	n, err := c.Read(buffer.FreeBytes())
	if err != nil {
		return M.Socksaddr{}, err
	}
	buffer.Truncate(n)

	var packetHeader []byte
	if c.method.udpCipher != nil {
		_, err = c.method.udpCipher.Open(buffer.Index(PacketNonceSize), buffer.To(PacketNonceSize), buffer.From(PacketNonceSize), nil)
		if err != nil {
			return M.Socksaddr{}, E.Cause(err, "decrypt packet")
		}
		buffer.Advance(PacketNonceSize)
		buffer.Truncate(buffer.Len() - c.method.udpCipher.Overhead())
	} else {
		packetHeader = buffer.To(aes.BlockSize)
		c.method.udpBlockCipher.Decrypt(packetHeader, packetHeader)
	}

	var sessionId, packetId uint64
	err = binary.Read(buffer, binary.BigEndian, &sessionId)
	if err != nil {
		return M.Socksaddr{}, err
	}
	err = binary.Read(buffer, binary.BigEndian, &packetId)
	if err != nil {
		return M.Socksaddr{}, err
	}

	var remoteCipher cipher.AEAD
	if packetHeader != nil {
		if sessionId == c.session.remoteSessionId {
			remoteCipher = c.session.remoteCipher
		} else if sessionId == c.session.lastRemoteSessionId {
			remoteCipher = c.session.lastRemoteCipher
		} else {
			key := Blake3DeriveKey(c.method.psk[:], packetHeader[:8], c.method.keyLength)
			remoteCipher = c.method.constructor(common.Dup(key))
			runtime.KeepAlive(key)
		}
		_, err = remoteCipher.Open(buffer.Index(0), packetHeader[4:16], buffer.Bytes(), nil)
		if err != nil {
			return M.Socksaddr{}, E.Cause(err, "decrypt packet")
		}
		buffer.Truncate(buffer.Len() - remoteCipher.Overhead())
	}

	var headerType byte
	headerType, err = buffer.ReadByte()
	if err != nil {
		return M.Socksaddr{}, err
	}
	if headerType != HeaderTypeServer {
		return M.Socksaddr{}, ErrBadHeaderType
	}

	var epoch uint64
	err = binary.Read(buffer, binary.BigEndian, &epoch)
	if err != nil {
		return M.Socksaddr{}, err
	}

	diff := int(math.Abs(float64(time.Now().Unix() - int64(epoch))))
	if diff > 30 {
		return M.Socksaddr{}, ErrBadTimestamp
	}

	if sessionId == c.session.remoteSessionId {
		if !c.session.filter.ValidateCounter(packetId, math.MaxUint64) {
			return M.Socksaddr{}, ErrPacketIdNotUnique
		}
	} else if sessionId == c.session.lastRemoteSessionId {
		if !c.session.lastFilter.ValidateCounter(packetId, math.MaxUint64) {
			return M.Socksaddr{}, ErrPacketIdNotUnique
		}
		remoteCipher = c.session.lastRemoteCipher
		c.session.lastRemoteSeen = time.Now().Unix()
	} else {
		if c.session.remoteSessionId != 0 {
			if time.Now().Unix()-c.session.lastRemoteSeen < 60 {
				return M.Socksaddr{}, ErrTooManyServerSessions
			} else {
				c.session.lastRemoteSessionId = c.session.remoteSessionId
				c.session.lastFilter = c.session.filter
				c.session.lastRemoteSeen = time.Now().Unix()
				c.session.lastRemoteCipher = c.session.remoteCipher
				c.session.filter = wgReplay.Filter{}
			}
		}
		c.session.remoteSessionId = sessionId
		c.session.remoteCipher = remoteCipher
		c.session.filter.ValidateCounter(packetId, math.MaxUint64)
	}

	var clientSessionId uint64
	err = binary.Read(buffer, binary.BigEndian, &clientSessionId)
	if err != nil {
		return M.Socksaddr{}, err
	}

	if clientSessionId != c.session.sessionId {
		return M.Socksaddr{}, ErrBadClientSessionId
	}

	var paddingLength uint16
	err = binary.Read(buffer, binary.BigEndian, &paddingLength)
	if err != nil {
		return M.Socksaddr{}, E.Cause(err, "read padding length")
	}
	buffer.Advance(int(paddingLength))

	destination, err := socks5.AddressSerializer.ReadAddrPort(buffer)
	if err != nil {
		return M.Socksaddr{}, err
	}
	return destination, nil
}

func (c *clientPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	buffer := buf.With(p)
	destination, err := c.ReadPacket(buffer)
	if err != nil {
		return
	}
	addr = destination.UDPAddr()
	n = copy(p, buffer.Bytes())
	return
}

func (c *clientPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	destination := M.SocksaddrFromNet(addr)
	var overHead int
	if c.method.udpCipher != nil {
		overHead = PacketNonceSize + c.method.udpCipher.Overhead()
	} else {
		overHead = c.session.cipher.Overhead()
	}
	overHead += 16 // packet header
	pskLen := len(c.method.pskList)
	if c.method.udpCipher == nil && pskLen > 1 {
		overHead += (pskLen - 1) * aes.BlockSize
	}
	overHead += 1 // header type
	overHead += 8 // timestamp
	overHead += 2 // padding length
	overHead += socks5.AddressSerializer.AddrPortLen(destination)

	_buffer := buf.Make(overHead + len(p))
	defer runtime.KeepAlive(_buffer)
	buffer := buf.With(common.Dup(_buffer))

	var dataIndex int
	if c.method.udpCipher != nil {
		common.Must1(buffer.ReadFullFrom(c.method.secureRNG, PacketNonceSize))
		if pskLen > 1 {
			panic("unsupported chacha extended header")
		}
		dataIndex = buffer.Len()
	} else {
		dataIndex = aes.BlockSize
	}

	common.Must(
		binary.Write(buffer, binary.BigEndian, c.session.sessionId),
		binary.Write(buffer, binary.BigEndian, c.session.nextPacketId()),
	)

	if c.method.udpCipher == nil && pskLen > 1 {
		for i, psk := range c.method.pskList {
			dataIndex += aes.BlockSize
			pskHash := c.method.pskHash[aes.BlockSize*i : aes.BlockSize*(i+1)]

			identityHeader := buffer.Extend(aes.BlockSize)
			for textI := 0; textI < aes.BlockSize; textI++ {
				identityHeader[textI] = pskHash[textI] ^ buffer.Byte(textI)
			}
			c.method.blockConstructor(psk[:]).Encrypt(identityHeader, identityHeader)

			if i == pskLen-2 {
				break
			}
		}
	}
	common.Must(
		buffer.WriteByte(HeaderTypeClient),
		binary.Write(buffer, binary.BigEndian, uint64(time.Now().Unix())),
		binary.Write(buffer, binary.BigEndian, uint16(0)), // padding length
	)
	err = socks5.AddressSerializer.WriteAddrPort(buffer, destination)
	if err != nil {
		return
	}
	if err != nil {
		return
	}
	if c.method.udpCipher != nil {
		c.method.udpCipher.Seal(buffer.Index(dataIndex), buffer.To(dataIndex), buffer.From(dataIndex), nil)
		buffer.Extend(c.method.udpCipher.Overhead())
	} else {
		packetHeader := buffer.To(aes.BlockSize)
		c.session.cipher.Seal(buffer.Index(dataIndex), packetHeader[4:16], buffer.From(dataIndex), nil)
		buffer.Extend(c.session.cipher.Overhead())
		c.method.udpBlockCipher.Encrypt(packetHeader, packetHeader)
	}
	err = common.Error(c.Write(buffer.Bytes()))
	if err != nil {
		return
	}
	return len(p), nil
}

type udpSession struct {
	headerType          byte
	sessionId           uint64
	packetId            uint64
	remoteSessionId     uint64
	lastRemoteSessionId uint64
	lastRemoteSeen      int64
	cipher              cipher.AEAD
	remoteCipher        cipher.AEAD
	lastRemoteCipher    cipher.AEAD
	filter              wgReplay.Filter
	lastFilter          wgReplay.Filter
}

func (s *udpSession) nextPacketId() uint64 {
	return atomic.AddUint64(&s.packetId, 1)
}

func (m *Method) newUDPSession() *udpSession {
	session := &udpSession{}
	common.Must(binary.Read(m.secureRNG, binary.BigEndian, &session.sessionId))
	session.packetId--
	if m.udpCipher == nil {
		sessionId := make([]byte, 8)
		binary.BigEndian.PutUint64(sessionId, session.sessionId)
		key := Blake3DeriveKey(m.psk[:], sessionId, m.keyLength)
		session.cipher = m.constructor(common.Dup(key))
		runtime.KeepAlive(key)
	}
	return session
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
