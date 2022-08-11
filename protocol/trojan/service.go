package trojan

import (
	"context"
	"io"
	"net"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	F "github.com/sagernet/sing/common/format"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/rw"
)

type Handler interface {
	N.TCPConnectionHandler
	N.UDPConnectionHandler
}

type Service[K comparable] struct {
	handler Handler
	users   map[K][56]byte
	keys    map[[56]byte]K
}

func NewService[K comparable](handler Handler) *Service[K] {
	return &Service[K]{
		handler: handler,
		users:   make(map[K][56]byte),
		keys:    make(map[[56]byte]K),
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
	_, err := io.ReadFull(conn, common.Dup(key[:]))
	if err != nil {
		return err
	}

	goto process

returnErr:
	err = &Error{
		Metadata: metadata,
		Conn:     conn,
		Inner:    err,
	}
	return err

process:

	if user, loaded := s.keys[key]; loaded {
		ctx = auth.ContextWithUser(ctx, user)
	} else {
		err = E.New("bad request")
		goto returnErr
	}

	err = rw.SkipN(conn, 2)
	if err != nil {
		err = E.Cause(err, "skip crlf")
		goto returnErr
	}

	command, err := rw.ReadByte(conn)
	if err != nil {
		err = E.Cause(err, "read command")
		goto returnErr
	}

	if command != CommandTCP && command != CommandUDP {
		err = E.New("unknown command ", command)
		goto returnErr
	}

	destination, err := M.SocksaddrSerializer.ReadAddrPort(conn)
	if err != nil {
		err = E.Cause(err, "read destination")
		goto returnErr
	}

	err = rw.SkipN(conn, 2)
	if err != nil {
		err = E.Cause(err, "skip crlf")
		goto returnErr
	}

	metadata.Protocol = "trojan"
	metadata.Destination = destination

	if command == CommandTCP {
		return s.handler.NewConnection(ctx, conn, metadata)
	} else {
		return s.handler.NewPacketConnection(ctx, &PacketConn{conn}, metadata)
	}
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

type Error struct {
	Metadata M.Metadata
	Conn     net.Conn
	Inner    error
}

func (e *Error) Error() string {
	return F.ToString("process connection from ", e.Metadata.Source, ": ", e.Inner)
}

func (e *Error) Unwrap() error {
	return e.Inner
}

func (e *Error) Close() error {
	return e.Conn.Close()
}
