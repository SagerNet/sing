package shadowsocks

import (
	"context"
	"fmt"
	"net"

	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type Service interface {
	M.TCPConnectionHandler
	N.UDPHandler
}

type Handler interface {
	M.TCPConnectionHandler
	N.UDPConnectionHandler
	E.Handler
}

type UserContext[U comparable] struct {
	context.Context
	User U
}

type ServerConnError struct {
	net.Conn
	Source M.Socksaddr
	Cause  error
}

func (e *ServerConnError) Unwrap() error {
	return e.Cause
}

func (e *ServerConnError) Error() string {
	return fmt.Sprint("shadowsocks: serve TCP from ", e.Source, ": ", e.Cause)
}

type ServerPacketError struct {
	N.PacketConn
	Source M.Socksaddr
	Cause  error
}

func (e *ServerPacketError) Unwrap() error {
	return e.Cause
}

func (e *ServerPacketError) Error() string {
	return fmt.Sprint("shadowsocks: serve UDP from ", e.Source, ": ", e.Cause)
}
