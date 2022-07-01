package network

import (
	"context"
	"net"
	"time"

	M "github.com/sagernet/sing/common/metadata"
)

type Dialer interface {
	DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error)
	ListenPacket(ctx context.Context) (net.PacketConn, error)
}

var SystemDialer Dialer = &DefaultDialer{
	Dialer: net.Dialer{
		Timeout: 5 * time.Second,
	},
}

type DefaultDialer struct {
	net.Dialer
	net.ListenConfig
}

func (d *DefaultDialer) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	return d.Dialer.DialContext(ctx, network, destination.String())
}

func (d *DefaultDialer) ListenPacket(ctx context.Context) (net.PacketConn, error) {
	return d.ListenConfig.ListenPacket(ctx, "udp", "")
}
