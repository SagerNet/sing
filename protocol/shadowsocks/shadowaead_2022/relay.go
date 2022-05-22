package shadowaead_2022

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"io"
	"net"
	"os"
	"runtime"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	"github.com/sagernet/sing/common/cache"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/udpnat"
	"github.com/sagernet/sing/protocol/shadowsocks"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead"
	"lukechampine.com/blake3"
)

type Relay[U comparable] struct {
	name          string
	secureRNG     io.Reader
	keySaltLength int
	handler       shadowsocks.Handler

	constructor      func(key []byte) cipher.AEAD
	blockConstructor func(key []byte) cipher.Block
	udpBlockCipher   cipher.Block

	iPSK         []byte
	uPSKHash     map[U][aes.BlockSize]byte
	uPSKHashR    map[[aes.BlockSize]byte]U
	uDestination map[U]M.Socksaddr
	uCipher      map[U]cipher.Block
	udpNat       *udpnat.Service[uint64]
	udpSessions  *cache.LruCache[uint64, *relayUDPSession]
}

func (s *Relay[U]) AddUser(user U, key []byte, destination M.Socksaddr) error {
	if len(key) < s.keySaltLength {
		return shadowsocks.ErrBadKey
	} else if len(key) > s.keySaltLength {
		key = Key(key, s.keySaltLength)
	}

	var uPSKHash [aes.BlockSize]byte
	hash512 := blake3.Sum512(key)
	copy(uPSKHash[:], hash512[:])

	if oldHash, loaded := s.uPSKHash[user]; loaded {
		delete(s.uPSKHashR, oldHash)
	}

	s.uPSKHash[user] = uPSKHash
	s.uPSKHashR[uPSKHash] = user
	s.uDestination[user] = destination
	s.uCipher[user] = s.blockConstructor(key)

	return nil
}

func (s *Relay[U]) RemoveUser(user U) {
	if hash, loaded := s.uPSKHash[user]; loaded {
		delete(s.uPSKHashR, hash)
	}
	delete(s.uPSKHash, user)
	delete(s.uCipher, user)
}

func NewRelay[U comparable](method string, psk []byte, secureRNG io.Reader, udpTimeout int64, handler shadowsocks.Handler) (*Relay[U], error) {
	s := &Relay[U]{
		name:      method,
		secureRNG: secureRNG,
		handler:   handler,

		uPSKHash:     make(map[U][aes.BlockSize]byte),
		uPSKHashR:    make(map[[aes.BlockSize]byte]U),
		uDestination: make(map[U]M.Socksaddr),
		uCipher:      make(map[U]cipher.Block),

		udpNat: udpnat.New[uint64](udpTimeout, handler),
		udpSessions: cache.New(
			cache.WithAge[uint64, *relayUDPSession](udpTimeout),
			cache.WithUpdateAgeOnGet[uint64, *relayUDPSession](),
		),
	}

	switch method {
	case "2022-blake3-aes-128-gcm":
		s.keySaltLength = 16
		s.constructor = newAESGCM
		s.blockConstructor = newAES
	case "2022-blake3-aes-256-gcm":
		s.keySaltLength = 32
		s.constructor = newAESGCM
		s.blockConstructor = newAES
	default:
		return nil, os.ErrInvalid
	}
	if len(psk) != s.keySaltLength {
		if len(psk) < s.keySaltLength {
			return nil, shadowsocks.ErrBadKey
		} else {
			psk = Key(psk, s.keySaltLength)
		}
	}
	s.udpBlockCipher = s.blockConstructor(psk)
	return s, nil
}

func (s *Relay[U]) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	err := s.newConnection(ctx, conn, metadata)
	if err != nil {
		err = &shadowsocks.ServerConnError{Conn: conn, Source: metadata.Source, Cause: err}
	}
	return err
}

func (s *Relay[U]) newConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	_requestHeader := buf.StackNew()
	defer runtime.KeepAlive(_requestHeader)
	requestHeader := common.Dup(_requestHeader)
	n, err := requestHeader.ReadFrom(conn)
	if err != nil {
		return err
	} else if int(n) < s.keySaltLength+aes.BlockSize {
		return shadowaead.ErrBadHeader
	}
	requestSalt := requestHeader.To(s.keySaltLength)
	var _eiHeader [aes.BlockSize]byte
	eiHeader := common.Dup(_eiHeader[:])
	copy(eiHeader, requestHeader.Range(s.keySaltLength, s.keySaltLength+aes.BlockSize))

	keyMaterial := buf.Make(s.keySaltLength * 2)
	copy(keyMaterial, s.iPSK)
	copy(keyMaterial[s.keySaltLength:], requestSalt)
	_identitySubkey := buf.Make(s.keySaltLength)
	identitySubkey := common.Dup(_identitySubkey)
	blake3.DeriveKey(identitySubkey, "shadowsocks 2022 identity subkey", keyMaterial)
	s.blockConstructor(identitySubkey).Decrypt(eiHeader, eiHeader)
	runtime.KeepAlive(_identitySubkey)

	var user U
	if u, loaded := s.uPSKHashR[_eiHeader]; loaded {
		user = u
	} else {
		return E.New("invalid request")
	}
	runtime.KeepAlive(_eiHeader)

	copy(requestHeader.Range(aes.BlockSize, aes.BlockSize+s.keySaltLength), requestHeader.To(s.keySaltLength))
	requestHeader.Advance(aes.BlockSize)

	ctx = shadowsocks.UserContext[U]{
		ctx,
		user,
	}
	metadata.Protocol = "shadowsocks-relay"
	metadata.Destination = s.uDestination[user]
	conn = &bufio.BufferedConn{
		Conn:   conn,
		Buffer: requestHeader,
	}
	return s.handler.NewConnection(ctx, conn, metadata)
}

func (s *Relay[U]) NewPacket(ctx context.Context, conn N.PacketConn, buffer *buf.Buffer, metadata M.Metadata) error {
	err := s.newPacket(ctx, conn, buffer, metadata)
	if err != nil {
		err = &shadowsocks.ServerPacketError{Source: metadata.Source, Cause: err}
	}
	return err
}

func (s *Relay[U]) newPacket(ctx context.Context, conn N.PacketConn, buffer *buf.Buffer, metadata M.Metadata) error {
	packetHeader := buffer.To(aes.BlockSize)
	s.udpBlockCipher.Decrypt(packetHeader, packetHeader)

	sessionId := binary.BigEndian.Uint64(packetHeader)

	var _eiHeader [aes.BlockSize]byte
	eiHeader := common.Dup(_eiHeader[:])
	s.udpBlockCipher.Decrypt(eiHeader, buffer.Range(aes.BlockSize, 2*aes.BlockSize))

	for i := range eiHeader {
		eiHeader[i] = eiHeader[i] ^ packetHeader[i]
	}

	var user U
	if u, loaded := s.uPSKHashR[_eiHeader]; loaded {
		user = u
	} else {
		return E.New("invalid request")
	}

	session, _ := s.udpSessions.LoadOrStore(sessionId, func() *relayUDPSession {
		return new(relayUDPSession)
	})
	session.sourceAddr = metadata.Source

	s.uCipher[user].Encrypt(packetHeader, packetHeader)
	copy(buffer.Range(aes.BlockSize, 2*aes.BlockSize), packetHeader)
	buffer.Advance(aes.BlockSize)

	metadata.Protocol = "shadowsocks-relay"
	metadata.Destination = s.uDestination[user]
	s.udpNat.NewContextPacket(ctx, sessionId, func() (context.Context, N.PacketWriter) {
		return &shadowsocks.UserContext[U]{
			ctx,
			user,
		}, &relayPacketWriter[U]{conn, session}
	}, buffer, metadata)
	return nil
}

type relayUDPSession struct {
	sourceAddr M.Socksaddr
}

type relayPacketWriter[U comparable] struct {
	N.PacketConn
	session *relayUDPSession
}

func (w *relayPacketWriter[U]) WritePacket(buffer *buf.Buffer, _ M.Socksaddr) error {
	return w.PacketConn.WritePacket(buffer, w.session.sourceAddr)
}
