package shadowsocks

import (
	"context"
	"fmt"
	"net"

	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/protocol/socks"
)

type Service interface {
	M.TCPConnectionHandler
	socks.UDPHandler
}

type Handler interface {
	M.TCPConnectionHandler
	socks.UDPConnectionHandler
	E.Handler
}

type MultiUserService[U comparable] interface {
	Service
	AddUser(user U, key []byte)
	RemoveUser(user U)
}

type UserContext[U comparable] struct {
	context.Context
	User U
}

type ServerConnError struct {
	net.Conn
	Source *M.AddrPort
	Cause  error
}

func (e *ServerConnError) Unwrap() error {
	return e.Cause
}

func (e *ServerConnError) Error() string {
	return fmt.Sprint("shadowsocks: serve TCP from ", e.Source, ": ", e.Cause)
}

type ServerPacketError struct {
	socks.PacketConn
	Source *M.AddrPort
	Cause  error
}

func (e *ServerPacketError) Unwrap() error {
	return e.Cause
}

func (e *ServerPacketError) Error() string {
	return fmt.Sprint("shadowsocks: serve UDP from ", e.Source, ": ", e.Cause)
}
