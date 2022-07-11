package network

import (
	"context"
	"net"

	M "github.com/sagernet/sing/common/metadata"
)

type Dialer interface {
	DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error)
	ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error)
}

var SystemDialer Dialer = &DefaultDialer{}

type DefaultDialer struct {
	net.Dialer
	net.ListenConfig
}

func (d *DefaultDialer) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	return d.Dialer.DialContext(ctx, network, destination.String())
}

func (d *DefaultDialer) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	return d.ListenConfig.ListenPacket(ctx, "udp", "")
}
