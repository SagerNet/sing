package trojan

import (
	"context"
	"crypto/tls"
	"net"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/rw"
)

type Handler interface {
	N.TCPConnectionHandler
	N.UDPConnectionHandler
}

type Service[K comparable] struct {
	users           map[K][56]byte
	keys            map[[56]byte]K
	handler         Handler
	fallbackHandler N.TCPConnectionHandler
}

func NewService[K comparable](handler Handler, fallbackHandler N.TCPConnectionHandler) *Service[K] {
	return &Service[K]{
		users:           make(map[K][56]byte),
		keys:            make(map[[56]byte]K),
		handler:         handler,
		fallbackHandler: fallbackHandler,
	}
}

var ErrUserExists = E.New("user already exists")

func (s *Service[K]) UpdateUsers(userList []K, passwordList []string) error {
	users := make(map[K][56]byte)
	keys := make(map[[56]byte]K)
	for i, user := range userList {
		if _, loaded := users[user]; loaded {
			return ErrUserExists
		}
		key := Key(passwordList[i])
		if oldUser, loaded := keys[key]; loaded {
			return E.Extend(ErrUserExists, "password used by ", oldUser)
		}
		users[user] = key
		keys[key] = user
	}
	s.users = users
	s.keys = keys
	return nil
}

func (s *Service[K]) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	var key [KeyLength]byte
	n, err := conn.Read(common.Dup(key[:]))
	if err != nil {
		return err
	} else if n != KeyLength {
		return s.fallback(ctx, conn, metadata, key[:n], E.New("bad request size"))
	}

	if user, loaded := s.keys[key]; loaded {
		ctx = auth.ContextWithUser(ctx, user)
	} else {
		return s.fallback(ctx, conn, metadata, key[:], E.New("bad request"))
	}

	err = rw.SkipN(conn, 2)
	if err != nil {
		return E.Cause(err, "skip crlf")
	}

	command, err := rw.ReadByte(conn)
	if err != nil {
		return E.Cause(err, "read command")
	}

	if command != CommandTCP && command != CommandUDP {
		return E.New("unknown command ", command)
	}

	destination, err := M.SocksaddrSerializer.ReadAddrPort(conn)
	if err != nil {
		return E.Cause(err, "read destination")
	}

	err = rw.SkipN(conn, 2)
	if err != nil {
		return E.Cause(err, "skip crlf")
	}

	metadata.Protocol = "trojan"
	metadata.Destination = destination

	if command == CommandTCP {
		return s.handler.NewConnection(ctx, conn, metadata)
	} else {
		return s.handler.NewPacketConnection(ctx, &PacketConn{conn}, metadata)
	}
}

func (s *Service[K]) fallback(ctx context.Context, conn net.Conn, metadata M.Metadata, header []byte, err error) error {
	if s.fallbackHandler == nil {
		return E.Extend(err, "fallback disabled")
	}
	if tlsConn, ok := conn.(*tls.Conn); ok {
		cs := tlsConn.ConnectionState()
		metadata.Alpn = cs.NegotiatedProtocol
	}
	conn = bufio.NewCachedConn(conn, buf.As(header).ToOwned())
	return s.fallbackHandler.NewConnection(ctx, conn, metadata)
}

type PacketConn struct {
	net.Conn
}

func (c *PacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	return ReadPacket(c.Conn, buffer)
}

func (c *PacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	return WritePacket(c.Conn, buffer, destination)
}

func (c *PacketConn) FrontHeadroom() int {
	return M.MaxSocksaddrLength + 4
}
