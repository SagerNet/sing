package core

import (
	"sing/common/session"
	"sing/transport"
)

type Instance interface {
	session.Handler
	transport.InboundManager
	transport.OutboundManager
}
