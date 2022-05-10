package socks

import (
	"context"
	"net"
	"net/netip"

	"github.com/sagernet/sing/common/auth"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/transport/tcp"
)

type Handler interface {
	tcp.Handler
	N.UDPConnectionHandler
}

type Listener struct {
	bindAddr      netip.Addr
	tcpListener   *tcp.Listener
	authenticator auth.Authenticator
	handler       Handler
}

func NewListener(bind netip.AddrPort, authenticator auth.Authenticator, handler Handler) *Listener {
	listener := &Listener{
		bindAddr:      bind.Addr(),
		handler:       handler,
		authenticator: authenticator,
	}
	listener.tcpListener = tcp.NewTCPListener(bind, listener)
	return listener
}

func (l *Listener) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	return HandleConnection(ctx, conn, l.authenticator, l.handler, metadata)
}

func (l *Listener) Start() error {
	return l.tcpListener.Start()
}

func (l *Listener) Close() error {
	return l.tcpListener.Close()
}

func (l *Listener) HandleError(err error) {
	l.handler.HandleError(err)
}
