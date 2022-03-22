package transport

import (
	"context"

	"sing/common/list"
)

type Inbound interface {
	Init(ctx *InboundContext)
	Start() error
	Close() error
}

type InboundContext struct {
	Context context.Context
	Tag     string
	Inbound Inbound
}

type InboundManager interface {
	AddInbound(inbound Inbound, tag string)
	Inbounds() *list.List[*InboundContext]
	Inbound(tag string) *InboundContext
}
