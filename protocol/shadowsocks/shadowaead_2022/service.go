package shadowaead_2022

import (
	"crypto/cipher"
	"encoding/binary"
	"io"
	"math"
	"net"
	"sync"
	"time"

	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/replay"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/protocol/shadowsocks"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead"
	"github.com/sagernet/sing/protocol/socks"
)

type Service struct {
	name         string
	secureRNG    io.Reader
	keyLength    int
	constructor  func(key []byte) cipher.AEAD
	psk          []byte
	replayFilter replay.Filter
	handler      shadowsocks.Handler
}

func NewService(method string, psk []byte, secureRNG io.Reader, handler shadowsocks.Handler) (shadowsocks.Service, error) {
	s := &Service{
		name:         method,
		psk:          psk,
		secureRNG:    secureRNG,
		replayFilter: replay.NewCuckoo(60),
		handler:      handler,
	}

	if len(psk) != KeySaltSize {
		return nil, shadowaead.ErrBadKey
	}

	switch method {
	case "2022-blake3-aes-128-gcm":
		s.keyLength = 16
		s.constructor = newAESGCM
		// m.blockConstructor = newAES
		// m.udpBlockCipher = newAES(m.psk)
	case "2022-blake3-aes-256-gcm":
		s.keyLength = 32
		s.constructor = newAESGCM
		// m.blockConstructor = newAES
		// m.udpBlockCipher = newAES(m.psk)
	case "2022-blake3-chacha20-poly1305":
		s.keyLength = 32
		s.constructor = newChacha20Poly1305
		// m.udpCipher = newXChacha20Poly1305(m.psk)
	}
	return s, nil
}

func (s *Service) NewConnection(conn net.Conn, metadata M.Metadata) error {
	requestSalt := make([]byte, KeySaltSize)
	_, err := io.ReadFull(conn, requestSalt)
	if err != nil {
		return E.Cause(err, "read request salt")
	}

	if !s.replayFilter.Check(requestSalt) {
		return E.New("salt not unique")
	}

	requestKey := Blake3DeriveKey(s.psk, requestSalt, s.keyLength)
	reader := shadowaead.NewReader(
		conn,
		s.constructor(common.Dup(requestKey)),
		MaxPacketSize,
	)

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

	destination, err := socks.AddressSerializer.ReadAddrPort(reader)
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
	return s.handler.NewConnection(&serverConn{
		Service:     s,
		Conn:        conn,
		reader:      reader,
		requestSalt: requestSalt,
	}, metadata)
}

type serverConn struct {
	*Service
	net.Conn
	access      sync.Mutex
	reader      *shadowaead.Reader
	writer      *shadowaead.Writer
	requestSalt []byte
}

func (c *serverConn) writeResponse(payload []byte) (n int, err error) {
	_salt := make([]byte, KeySaltSize)
	salt := common.Dup(_salt)
	common.Must1(io.ReadFull(c.secureRNG, salt))
	key := Blake3DeriveKey(c.psk, salt, c.keyLength)
	writer := shadowaead.NewWriter(
		c.Conn,
		c.constructor(common.Dup(key)),
		MaxPacketSize,
	)
	header := writer.Buffer()
	header.Write(salt)
	bufferedWriter := writer.BufferedWriter(header.Len())

	common.Must(rw.WriteByte(bufferedWriter, HeaderTypeServer))
	common.Must(binary.Write(bufferedWriter, binary.BigEndian, uint64(time.Now().Unix())))
	common.Must1(bufferedWriter.Write(c.requestSalt))
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
	if c.writer != nil {
		return rw.ReadFrom0(c, r)
	}
	return c.writer.ReadFrom(r)
}

func (c *serverConn) WriteTo(w io.Writer) (n int64, err error) {
	return c.reader.WriteTo(w)
}
