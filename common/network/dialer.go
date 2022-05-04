package network

import (
	"context"
	"net"

	M "github.com/sagernet/sing/common/metadata"
)

type ContextDialer interface {
	DialContext(ctx context.Context, network string, address M.Socksaddr) (net.Conn, error)
}

var SystemDialer ContextDialer = &DefaultDialer{}

type DefaultDialer struct {
	net.Dialer
}

func (d *DefaultDialer) ListenUDP(network string, laddr *net.UDPAddr) (*net.UDPConn, error) {
	return net.ListenUDP(network, laddr)
}

func (d *DefaultDialer) DialContext(ctx context.Context, network string, address M.Socksaddr) (net.Conn, error) {
	return d.Dialer.DialContext(ctx, network, address.String())
}

type Listener interface {
	Listen(ctx context.Context, network, address string) (net.Listener, error)
	ListenPacket(ctx context.Context, network, address string) (net.PacketConn, error)
}

var SystemListener Listener = &net.ListenConfig{}
