package network

import (
	"context"
	"net"
)

type ContextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

var SystemDialer ContextDialer = &net.Dialer{}
