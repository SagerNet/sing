package network

import (
	"context"
	"net"

	M "github.com/sagernet/sing/common/metadata"
)

type ContextDialer interface {
	DialContext(ctx context.Context, network string, address *M.AddrPort) (net.Conn, error)
}

var SystemDialer ContextDialer = &DefaultDialer{}

type DefaultDialer struct {
	net.Dialer
}

func (d *DefaultDialer) DialContext(ctx context.Context, network string, address *M.AddrPort) (net.Conn, error) {
	return d.Dialer.DialContext(ctx, network, address.String())
}
