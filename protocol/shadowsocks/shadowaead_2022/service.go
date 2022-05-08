package shadowaead_2022

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"io"
	"math"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/cache"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/replay"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/common/udpnat"
	"github.com/sagernet/sing/protocol/shadowsocks"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead"
	"github.com/sagernet/sing/protocol/socks5"
	wgReplay "golang.zx2c4.com/wireguard/replay"
)

type Service struct {
	name             string
	secureRNG        io.Reader
	keyLength        int
	constructor      func(key []byte) cipher.AEAD
	blockConstructor func(key []byte) cipher.Block
	udpCipher        cipher.AEAD
	udpBlockCipher   cipher.Block
	psk              [KeySaltSize]byte
	replayFilter     replay.Filter
	handler          shadowsocks.Handler
	udpNat           *udpnat.Service[uint64]
	sessions         *cache.LruCache[uint64, *serverUDPSession]
}

func NewService(method string, psk [KeySaltSize]byte, secureRNG io.Reader, udpTimeout int64, handler shadowsocks.Handler) (shadowsocks.Service, error) {
	s := &Service{
		name:         method,
		psk:          psk,
		secureRNG:    secureRNG,
		replayFilter: replay.NewCuckoo(60),
		handler:      handler,
		udpNat:       udpnat.New[uint64](udpTimeout, handler),
		sessions: cache.New[uint64, *serverUDPSession](
			cache.WithAge[uint64, *serverUDPSession](udpTimeout),
			cache.WithUpdateAgeOnGet[uint64, *serverUDPSession](),
		),
	}

	switch method {
	case "2022-blake3-aes-128-gcm":
		s.keyLength = 16
		s.constructor = newAESGCM
		s.blockConstructor = newAES
		s.udpBlockCipher = newAES(s.psk[:])
	case "2022-blake3-aes-256-gcm":
		s.keyLength = 32
		s.constructor = newAESGCM
		s.blockConstructor = newAES
		s.udpBlockCipher = newAES(s.psk[:])
	case "2022-blake3-chacha20-poly1305":
		s.keyLength = 32
		s.constructor = newChacha20Poly1305
		s.udpCipher = newXChacha20Poly1305(s.psk[:])
	}

	return s, nil
}

func (s *Service) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	err := s.newConnection(ctx, conn, metadata)
	if err != nil {
		err = &shadowsocks.ServerConnError{Conn: conn, Source: metadata.Source, Cause: err}
	}
	return err
}

func (s *Service) newConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	requestSalt := make([]byte, KeySaltSize)
	_, err := io.ReadFull(conn, requestSalt)
	if err != nil {
		return E.Cause(err, "read request salt")
	}

	if !s.replayFilter.Check(requestSalt) {
		return E.New("salt not unique")
	}

	requestKey := Blake3DeriveKey(s.psk[:], requestSalt, s.keyLength)
	reader := shadowaead.NewReader(
		conn,
		s.constructor(common.Dup(requestKey)),
		MaxPacketSize,
	)
	runtime.KeepAlive(requestKey)

	headerType, err := rw.ReadByte(reader)
	if err != nil {
		return E.Cause(err, "read header")
	}

	if headerType != HeaderTypeClient {
		return ErrBadHeaderType
	}

	var epoch uint64
	err = binary.Read(reader, binary.BigEndian, &epoch)
	if err != nil {
		return E.Cause(err, "read timestamp")
	}

	if math.Abs(float64(time.Now().Unix()-int64(epoch))) > 30 {
		return ErrBadTimestamp
	}

	destination, err := socks5.AddressSerializer.ReadAddrPort(reader)
	if err != nil {
		return E.Cause(err, "read destination")
	}

	var paddingLen uint16
	err = binary.Read(reader, binary.BigEndian, &paddingLen)
	if err != nil {
		return E.Cause(err, "read padding length")
	}

	if paddingLen > 0 {
		err = reader.Discard(int(paddingLen))
		if err != nil {
			return E.Cause(err, "discard padding")
		}
	}

	metadata.Protocol = "shadowsocks"
	metadata.Destination = destination
	return s.handler.NewConnection(ctx, &serverConn{
		Service:     s,
		Conn:        conn,
		uPSK:        s.psk,
		reader:      reader,
		requestSalt: requestSalt,
	}, metadata)
}

type serverConn struct {
	*Service
	net.Conn
	uPSK        [KeySaltSize]byte
	access      sync.Mutex
	reader      *shadowaead.Reader
	writer      *shadowaead.Writer
	requestSalt []byte
}

func (c *serverConn) writeResponse(payload []byte) (n int, err error) {
	var _salt [KeySaltSize]byte
	salt := common.Dup(_salt[:])
	common.Must1(io.ReadFull(c.secureRNG, salt))
	key := Blake3DeriveKey(c.uPSK[:], salt, c.keyLength)
	runtime.KeepAlive(_salt)
	writer := shadowaead.NewWriter(
		c.Conn,
		c.constructor(common.Dup(key)),
		MaxPacketSize,
	)
	runtime.KeepAlive(key)
	header := writer.Buffer()
	header.Write(salt)
	bufferedWriter := writer.BufferedWriter(header.Len())

	common.Must(rw.WriteByte(bufferedWriter, HeaderTypeServer))
	common.Must(binary.Write(bufferedWriter, binary.BigEndian, uint64(time.Now().Unix())))
	common.Must1(bufferedWriter.Write(c.requestSalt[:]))
	c.requestSalt = nil

	if len(payload) > 0 {
		_, err = bufferedWriter.Write(payload)
		if err != nil {
			return
		}
	}

	err = bufferedWriter.Flush()
	if err != nil {
		return
	}

	c.writer = writer
	n = len(payload)
	return
}

func (c *serverConn) Write(p []byte) (n int, err error) {
	if c.writer != nil {
		return c.writer.Write(p)
	}
	c.access.Lock()
	if c.writer != nil {
		c.access.Unlock()
		return c.writer.Write(p)
	}
	defer c.access.Unlock()
	return c.writeResponse(p)
}

func (c *serverConn) ReadFrom(r io.Reader) (n int64, err error) {
	if c.writer == nil {
		return rw.ReadFrom0(c, r)
	}
	return c.writer.ReadFrom(r)
}

func (c *serverConn) WriteTo(w io.Writer) (n int64, err error) {
	return c.reader.WriteTo(w)
}

func (c *serverConn) UpstreamReader() io.Reader {
	if c.reader == nil {
		return c.Conn
	}
	return c.reader
}

func (c *serverConn) ReaderReplaceable() bool {
	return c.reader != nil
}

func (c *serverConn) UpstreamWriter() io.Writer {
	if c.writer == nil {
		return c.Conn
	}
	return c.writer
}

func (c *serverConn) WriterReplaceable() bool {
	return c.writer != nil
}

func (s *Service) NewPacket(conn N.PacketConn, buffer *buf.Buffer, metadata M.Metadata) error {
	err := s.newPacket(conn, buffer, metadata)
	if err != nil {
		err = &shadowsocks.ServerPacketError{PacketConn: conn, Source: metadata.Source, Cause: err}
	}
	return err
}

func (s *Service) newPacket(conn N.PacketConn, buffer *buf.Buffer, metadata M.Metadata) error {
	var packetHeader []byte
	if s.udpCipher != nil {
		_, err := s.udpCipher.Open(buffer.Index(PacketNonceSize), buffer.To(PacketNonceSize), buffer.From(PacketNonceSize), nil)
		if err != nil {
			return E.Cause(err, "decrypt packet header")
		}
		buffer.Advance(PacketNonceSize)
	} else {
		packetHeader = buffer.To(aes.BlockSize)
		s.udpBlockCipher.Decrypt(packetHeader, packetHeader)
	}

	var sessionId, packetId uint64
	err := binary.Read(buffer, binary.BigEndian, &sessionId)
	if err != nil {
		return err
	}
	err = binary.Read(buffer, binary.BigEndian, &packetId)
	if err != nil {
		return err
	}

	session, loaded := s.sessions.LoadOrStore(sessionId, s.newUDPSession)
	if !loaded {
		session.remoteSessionId = sessionId
		if packetHeader != nil {
			key := Blake3DeriveKey(s.psk[:], packetHeader[:8], s.keyLength)
			session.remoteCipher = s.constructor(common.Dup(key))
			runtime.KeepAlive(key)
		}
	}
	goto process

returnErr:
	if !loaded {
		s.sessions.Delete(sessionId)
	}
	return err

process:
	if !session.filter.ValidateCounter(packetId, math.MaxUint64) {
		err = ErrPacketIdNotUnique
		goto returnErr
	}

	if packetHeader != nil {
		_, err = session.remoteCipher.Open(buffer.Index(0), packetHeader[4:16], buffer.Bytes(), nil)
		if err != nil {
			err = E.Cause(err, "decrypt packet")
			goto returnErr
		}
	}

	var headerType byte
	headerType, err = buffer.ReadByte()
	if err != nil {
		err = E.Cause(err, "decrypt packet")
		goto returnErr
	}
	if headerType != HeaderTypeClient {
		err = ErrBadHeaderType
		goto returnErr
	}

	var epoch uint64
	err = binary.Read(buffer, binary.BigEndian, &epoch)
	if err != nil {
		goto returnErr
	}
	if math.Abs(float64(uint64(time.Now().Unix())-epoch)) > 30 {
		err = ErrBadTimestamp
		goto returnErr
	}

	var paddingLength uint16
	err = binary.Read(buffer, binary.BigEndian, &paddingLength)
	if err != nil {
		err = E.Cause(err, "read padding length")
		goto returnErr
	}
	buffer.Advance(int(paddingLength))

	destination, err := socks5.AddressSerializer.ReadAddrPort(buffer)
	if err != nil {
		goto returnErr
	}
	metadata.Destination = destination

	session.remoteAddr = metadata.Source
	s.udpNat.NewPacket(sessionId, func() N.PacketWriter {
		return &serverPacketWriter{s, conn, session}
	}, buffer, metadata)
	return nil
}

type serverPacketWriter struct {
	*Service
	N.PacketConn
	session *serverUDPSession
}

func (w *serverPacketWriter) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	defer buffer.Release()

	_header := buf.StackNew()
	defer runtime.KeepAlive(_header)
	header := common.Dup(_header)

	var dataIndex int
	if w.udpCipher != nil {
		common.Must1(header.ReadFullFrom(w.secureRNG, PacketNonceSize))
		dataIndex = buffer.Len()
	} else {
		dataIndex = aes.BlockSize
	}

	common.Must(
		binary.Write(header, binary.BigEndian, w.session.sessionId),
		binary.Write(header, binary.BigEndian, w.session.nextPacketId()),
		header.WriteByte(HeaderTypeServer),
		binary.Write(header, binary.BigEndian, uint64(time.Now().Unix())),
		binary.Write(header, binary.BigEndian, w.session.remoteSessionId),
		binary.Write(header, binary.BigEndian, uint16(0)), // padding length
	)

	err := socks5.AddressSerializer.WriteAddrPort(header, destination)
	if err != nil {
		return err
	}

	_, err = header.Write(buffer.Bytes())
	if err != nil {
		return err
	}

	if w.udpCipher != nil {
		w.udpCipher.Seal(header.Index(dataIndex), header.To(dataIndex), header.From(dataIndex), nil)
		header.Extend(w.udpCipher.Overhead())
	} else {
		packetHeader := header.To(aes.BlockSize)
		w.session.cipher.Seal(header.Index(dataIndex), packetHeader[4:16], header.From(dataIndex), nil)
		header.Extend(w.session.cipher.Overhead())
		w.udpBlockCipher.Encrypt(packetHeader, packetHeader)
	}
	return w.PacketConn.WritePacket(header, w.session.remoteAddr)
}

type serverUDPSession struct {
	sessionId       uint64
	remoteSessionId uint64
	remoteAddr      M.Socksaddr
	packetId        uint64
	cipher          cipher.AEAD
	remoteCipher    cipher.AEAD
	filter          wgReplay.Filter
}

func (s *serverUDPSession) nextPacketId() uint64 {
	return atomic.AddUint64(&s.packetId, 1)
}

func (m *Service) newUDPSession() *serverUDPSession {
	session := &serverUDPSession{}
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
