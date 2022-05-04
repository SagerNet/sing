package tun

import (
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type Handler interface {
	M.TCPConnectionHandler
	N.UDPConnectionHandler
	E.Handler
}

type Stack interface {
	Start() error
	Close() error
}
