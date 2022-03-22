package transport

import (
	"context"

	"sing/common/list"
	"sing/common/session"
)

type Outbound interface {
	Init(ctx *OutboundContext)
	Close() error
	NewConnection(ctx context.Context, conn *session.Conn) error
	NewPacketConnection(ctx context.Context, packetConn *session.PacketConn) error
}

type OutboundContext struct {
	Context  context.Context
	Tag      string
	Outbound Outbound
}

type OutboundManager interface {
	AddOutbound(outbound Outbound, tag string)
	Outbounds() *list.List[*OutboundContext]
	Outbound(tag string) *OutboundContext
	DefaultOutbound() *OutboundContext
}
