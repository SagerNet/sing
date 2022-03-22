package sing

import (
	"context"
	"sing/common/session"
	"sync"

	"sing/common/gsync"
	"sing/common/list"
	"sing/core"
	"sing/transport"
)

var _ core.Instance = (*Instance)(nil)

type Instance struct {
	access          sync.Mutex
	ctx             context.Context
	cancel          context.CancelFunc
	inbounds        list.List[*transport.InboundContext]
	inboundByName   gsync.Map[string, *transport.InboundContext]
	outbounds       list.List[*transport.OutboundContext]
	outboundByName  gsync.Map[string, *transport.OutboundContext]
	defaultOutbound *transport.OutboundContext
}

func (i *Instance) AddInbound(inbound transport.Inbound, tag string) {
	i.access.Lock()
	defer i.access.Unlock()

	ic := new(transport.InboundContext)
	ic.Context = i.ctx
	ic.Tag = tag
	ic.Inbound = inbound

	i.inbounds.InsertAfter(ic)
	i.inboundByName.Store(tag, ic)
}

func (i *Instance) Inbounds() *list.List[*transport.InboundContext] {
	i.inboundByName.Range(func(tag string, inbound *transport.InboundContext) bool {
		return true
	})
	return &i.inbounds
}

func (i *Instance) Inbound(tag string) *transport.InboundContext {
	inbound, _ := i.inboundByName.Load(tag)
	return inbound
}

func (i *Instance) Outbounds() *list.List[*transport.OutboundContext] {
	return &i.outbounds
}

func (i *Instance) DefaultOutbound() *transport.OutboundContext {
	i.access.Lock()
	defer i.access.Unlock()
	return i.defaultOutbound
}

func (i *Instance) Outbound(tag string) *transport.OutboundContext {
	outbound, _ := i.outboundByName.Load(tag)
	return outbound
}

func (i *Instance) HandleConnection(conn *session.Conn) {
	i.defaultOutbound.Outbound.NewConnection(i.ctx, conn)
}

func (i *Instance) HandlePacket(packet *session.Packet) {
}

type InstanceContext interface {
	context.Context
	Instance() *Instance
	Load(key string) (any, bool)
	Store(key string, value any)
}

type instanceContext struct {
	context.Context
	instance Instance
	values   gsync.Map[any, string]
}

func (i *instanceContext) Load(key string) (any, bool) {
	return i.values.Load(key)
}

func (i *instanceContext) Store(key string, value any) {
	// TODO implement me
	panic("implement me")
}
