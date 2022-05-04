package shadowaead

import (
	"context"
	"crypto/cipher"
	"io"
	"net"
	"net/netip"
	"sync"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/replay"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/common/udpnat"
	"github.com/sagernet/sing/protocol/shadowsocks"
	"github.com/sagernet/sing/protocol/socks5"
	"golang.org/x/crypto/chacha20poly1305"
)

type Service struct {
	name          string
	keySaltLength int
	constructor   func(key []byte) cipher.AEAD
	key           []byte
	secureRNG     io.Reader
	replayFilter  replay.Filter
	handler       shadowsocks.Handler
	udpNat        *udpnat.Service[netip.AddrPort]
}

func NewService(method string, key []byte, password []byte, secureRNG io.Reader, replayFilter bool, udpTimeout int64, handler shadowsocks.Handler) (shadowsocks.Service, error) {
	s := &Service{
		name:      method,
		secureRNG: secureRNG,
		handler:   handler,
		udpNat:    udpnat.New[netip.AddrPort](udpTimeout, handler),
	}
	if replayFilter {
		s.replayFilter = replay.NewBloomRing()
	}
	switch method {
	case "aes-128-gcm":
		s.keySaltLength = 16
		s.constructor = newAESGCM
	case "aes-192-gcm":
		s.keySaltLength = 24
		s.constructor = newAESGCM
	case "aes-256-gcm":
		s.keySaltLength = 32
		s.constructor = newAESGCM
	case "chacha20-ietf-poly1305":
		s.keySaltLength = 32
		s.constructor = func(key []byte) cipher.AEAD {
			cipher, err := chacha20poly1305.New(key)
			common.Must(err)
			return cipher
		}
	case "xchacha20-ietf-poly1305":
		s.keySaltLength = 32
		s.constructor = func(key []byte) cipher.AEAD {
			cipher, err := chacha20poly1305.NewX(key)
			common.Must(err)
			return cipher
		}
	}
	if len(key) == s.keySaltLength {
		s.key = key
	} else if len(key) > 0 {
		return nil, ErrBadKey
	} else if len(password) > 0 {
		s.key = shadowsocks.Key(password, s.keySaltLength)
	} else {
		return nil, ErrMissingPassword
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
	_salt := buf.Make(s.keySaltLength)
	salt := common.Dup(_salt)

	_, err := io.ReadFull(conn, salt)
	if err != nil {
		return E.Cause(err, "read salt")
	}

	key := Kdf(s.key, salt, s.keySaltLength)
	reader := NewReader(conn, s.constructor(common.Dup(key)), MaxPacketSize)
	destination, err := socks5.AddressSerializer.ReadAddrPort(reader)
	if err != nil {
		return err
	}

	metadata.Protocol = "shadowsocks"
	metadata.Destination = destination

	return s.handler.NewConnection(ctx, &serverConn{
		Service: s,
		Conn:    conn,
		reader:  reader,
	}, metadata)
}

type serverConn struct {
	*Service
	net.Conn
	access sync.Mutex
	reader *Reader
	writer *Writer
}

func (c *serverConn) writeResponse(payload []byte) (n int, err error) {
	_salt := buf.Make(c.keySaltLength)
	salt := common.Dup(_salt)
	common.Must1(io.ReadFull(c.secureRNG, salt))

	key := Kdf(c.key, salt, c.keySaltLength)
	writer := NewWriter(
		c.Conn,
		c.constructor(common.Dup(key)),
		MaxPacketSize,
	)

	header := writer.Buffer()
	header.Write(salt)

	bufferedWriter := writer.BufferedWriter(header.Len())
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
	if buffer.Len() < s.keySaltLength {
		return E.New("bad packet")
	}
	key := Kdf(s.key, buffer.To(s.keySaltLength), s.keySaltLength)
	c := s.constructor(common.Dup(key))
	packet, err := c.Open(buffer.Index(s.keySaltLength), rw.ZeroBytes[:c.NonceSize()], buffer.From(s.keySaltLength), nil)
	if err != nil {
		return err
	}
	buffer.Advance(s.keySaltLength)
	buffer.Truncate(len(packet))
	metadata.Protocol = "shadowsocks"
	s.udpNat.NewPacket(metadata.Source.AddrPort(), func() N.PacketWriter {
		return &serverPacketWriter{s, conn, metadata.Source}
	}, buffer, metadata)
	return nil
}

type serverPacketWriter struct {
	*Service
	N.PacketConn
	source M.Socksaddr
}

func (w *serverPacketWriter) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	header := buffer.ExtendHeader(w.keySaltLength + socks5.AddressSerializer.AddrPortLen(destination))
	common.Must1(io.ReadFull(w.secureRNG, header[:w.keySaltLength]))
	err := socks5.AddressSerializer.WriteAddrPort(buf.With(header[w.keySaltLength:]), destination)
	if err != nil {
		return err
	}
	key := Kdf(w.key, buffer.To(w.keySaltLength), w.keySaltLength)
	c := w.constructor(common.Dup(key))
	c.Seal(buffer.From(w.keySaltLength)[:0], rw.ZeroBytes[:c.NonceSize()], buffer.From(w.keySaltLength), nil)
	buffer.Extend(c.Overhead())
	return w.PacketConn.WritePacket(buffer, w.source)
}
