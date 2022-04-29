package trojan

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/protocol/socks"
)

type Handler interface {
	M.TCPConnectionHandler
	socks.UDPConnectionHandler
}

type Context struct {
	context.Context
	User string
	Key  [KeyLength]byte
}

type Service struct {
	handler Handler
	keys    map[[56]byte]string
	users   map[string][56]byte
}

func NewService(handler Handler) *Service {
	return &Service{handler: handler}
}

var ErrUserExists = E.New("user already exists")

func (s *Service) AddUser(user string, password string) error {
	if _, loaded := s.users[user]; loaded {
		return ErrUserExists
	}
	key := Key(password)
	if oldUser, loaded := s.keys[key]; loaded {
		return E.New("password used by ", oldUser)
	}
	s.users[user] = key
	s.keys[key] = user
	return nil
}

func (s *Service) RemoveUser(user string) bool {
	if key, loaded := s.users[user]; loaded {
		delete(s.users, user)
		delete(s.keys, key)
		return true
	} else {
		return false
	}
}

func (s *Service) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
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

	var userCtx Context
	userCtx.Context = ctx
	if user, loaded := s.keys[key]; loaded {
		userCtx.User = user
		userCtx.Key = key
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

	destination, err := socks.AddressSerializer.ReadAddrPort(conn)
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
		return s.handler.NewConnection(userCtx, conn, metadata)
	} else {
		return s.handler.NewPacketConnection(userCtx, &packetConn{conn}, metadata)
	}
}

type packetConn struct {
	net.Conn
}

func (c *packetConn) ReadPacket(buffer *buf.Buffer) (*M.AddrPort, error) {
	return ReadPacket(c, buffer)
}

func (c *packetConn) WritePacket(buffer *buf.Buffer, destination *M.AddrPort) error {
	return WritePacket(c, buffer, destination)
}

type Error struct {
	Metadata M.Metadata
	Conn     net.Conn
	Inner    error
}

func (e *Error) Error() string {
	return fmt.Sprint("process connection from ", e.Metadata.Source, ": ", e.Inner.Error())
}

func (e *Error) Unwrap() error {
	return e.Inner
}

func (e *Error) Close() error {
	return e.Conn.Close()
}
