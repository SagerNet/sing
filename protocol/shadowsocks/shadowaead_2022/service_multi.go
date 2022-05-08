package shadowaead_2022

import (
	"context"
	"crypto/aes"
	"encoding/binary"
	"io"
	"math"
	"net"
	"runtime"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/protocol/shadowsocks"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead"
	"github.com/sagernet/sing/protocol/socks5"
	"lukechampine.com/blake3"
)

type MultiService[U comparable] struct {
	*Service

	uPSK      map[U][KeySaltSize]byte
	uPSKHash  map[U][aes.BlockSize]byte
	uPSKHashR map[[aes.BlockSize]byte]U
}

func (s *MultiService[U]) AddUser(user U, key []byte) {
	var uPSKHash [aes.BlockSize]byte
	hash512 := blake3.Sum512(key)
	copy(uPSKHash[:], hash512[:])

	if oldHash, loaded := s.uPSKHash[user]; loaded {
		delete(s.uPSKHashR, oldHash)
	}

	s.uPSKHash[user] = uPSKHash
	s.uPSKHashR[uPSKHash] = user

	var uPSK [KeySaltSize]byte
	copy(uPSK[:], key)
	s.uPSK[user] = uPSK
}

func (s *MultiService[U]) RemoveUser(user U) {
	if hash, loaded := s.uPSKHash[user]; loaded {
		delete(s.uPSKHashR, hash)
	}
	delete(s.uPSK, user)
	delete(s.uPSKHash, user)
}

func NewMultiService[U comparable](method string, iPSK [KeySaltSize]byte, secureRNG io.Reader, udpTimeout int64, handler shadowsocks.Handler) (shadowsocks.MultiUserService[U], error) {
	switch method {
	case "2022-blake3-aes-128-gcm":
	case "2022-blake3-aes-256-gcm":
	default:
		return nil, E.New("unsupported method ", method)
	}

	ss, err := NewService(method, iPSK, secureRNG, udpTimeout, handler)
	if err != nil {
		return nil, err
	}

	s := &MultiService[U]{
		Service: ss.(*Service),
	}
	return s, nil
}

func (s *MultiService[U]) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	err := s.newConnection(ctx, conn, metadata)
	if err != nil {
		err = &shadowsocks.ServerConnError{Conn: conn, Source: metadata.Source, Cause: err}
	}
	return err
}

func (s *MultiService[U]) newConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	requestSalt := make([]byte, KeySaltSize)
	_, err := io.ReadFull(conn, requestSalt)
	if err != nil {
		return E.Cause(err, "read request salt")
	}

	if !s.replayFilter.Check(requestSalt) {
		return E.New("salt not unique")
	}

	var _eiHeader [aes.BlockSize]byte
	eiHeader := common.Dup(_eiHeader[:])
	_, err = io.ReadFull(conn, eiHeader)
	if err != nil {
		return E.Cause(err, "read extended identity header")
	}

	keyMaterial := make([]byte, 2*KeySaltSize)
	copy(keyMaterial, s.psk[:])
	copy(keyMaterial[KeySaltSize:], requestSalt)
	_identitySubkey := buf.Make(s.keyLength)
	identitySubkey := common.Dup(_identitySubkey)
	blake3.DeriveKey(identitySubkey, "shadowsocks 2022 identity subkey", keyMaterial)
	s.blockConstructor(identitySubkey).Decrypt(eiHeader, eiHeader)
	runtime.KeepAlive(_identitySubkey)

	var user U
	var uPSK [KeySaltSize]byte
	if u, loaded := s.uPSKHashR[_eiHeader]; loaded {
		user = u
		uPSK = s.uPSK[u]
	} else {
		return E.New("invalid request")
	}

	requestKey := Blake3DeriveKey(uPSK[:], requestSalt, s.keyLength)
	reader := shadowaead.NewReader(
		conn,
		s.constructor(common.Dup(requestKey)),
		MaxPacketSize,
	)
	runtime.KeepAlive(requestSalt)

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
	diff := int(math.Abs(float64(time.Now().Unix() - int64(epoch))))
	if diff > 30 {
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

	var userCtx shadowsocks.UserContext[U]
	userCtx.Context = ctx
	userCtx.User = user

	metadata.Protocol = "shadowsocks"
	metadata.Destination = destination
	return s.handler.NewConnection(&userCtx, &serverConn{
		Service:     s.Service,
		Conn:        conn,
		uPSK:        uPSK,
		reader:      reader,
		requestSalt: requestSalt,
	}, metadata)
}

func (s *MultiService[U]) NewPacket(conn N.PacketConn, buffer *buf.Buffer, metadata M.Metadata) error {
	err := s.newPacket(conn, buffer, metadata)
	if err != nil {
		err = &shadowsocks.ServerPacketError{PacketConn: conn, Source: metadata.Source, Cause: err}
	}
	return err
}

func (s *MultiService[U]) newPacket(conn N.PacketConn, buffer *buf.Buffer, metadata M.Metadata) error {
	packetHeader := buffer.To(aes.BlockSize)
	s.udpBlockCipher.Decrypt(packetHeader, packetHeader)

	var _eiHeader [aes.BlockSize]byte
	eiHeader := common.Dup(_eiHeader[:])
	s.udpBlockCipher.Decrypt(eiHeader, buffer.Range(aes.BlockSize, 2*aes.BlockSize))

	for i := range eiHeader {
		eiHeader[i] = eiHeader[i] ^ packetHeader[i]
	}

	var user U
	var uPSK [KeySaltSize]byte
	if u, loaded := s.uPSKHashR[_eiHeader]; loaded {
		user = u
		uPSK = s.uPSK[u]
	} else {
		return E.New("invalid request")
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

	session, loaded := s.sessions.LoadOrStore(sessionId, func() *serverUDPSession {
		return s.newUDPSession(uPSK)
	})
	if !loaded {
		session.remoteSessionId = sessionId
		key := Blake3DeriveKey(uPSK[:], packetHeader[:8], s.keyLength)
		session.remoteCipher = s.constructor(common.Dup(key))
		runtime.KeepAlive(key)
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
		buffer.Truncate(buffer.Len() - session.remoteCipher.Overhead())
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

	var userCtx shadowsocks.UserContext[U]
	userCtx.Context = context.Background()
	userCtx.User = user

	s.udpNat.NewContextPacket(&userCtx, sessionId, func() N.PacketWriter {
		return &serverPacketWriter{s.Service, conn, session}
	}, buffer, metadata)
	return nil
}

func (m *MultiService[U]) newUDPSession(uPSK [KeySaltSize]byte) *serverUDPSession {
	session := &serverUDPSession{}
	common.Must(binary.Read(m.secureRNG, binary.BigEndian, &session.sessionId))
	session.packetId--
	sessionId := make([]byte, 8)
	binary.BigEndian.PutUint64(sessionId, session.sessionId)
	key := Blake3DeriveKey(uPSK[:], sessionId, m.keyLength)
	session.cipher = m.constructor(common.Dup(key))
	runtime.KeepAlive(key)
	return session
}
