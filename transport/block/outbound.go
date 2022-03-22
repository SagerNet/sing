package block

import (
	"context"

	"sing/common/session"
	"sing/transport"
)

var _ transport.Outbound = (*Outbound)(nil)

type Outbound struct {
}

func (h *Outbound) Init(*transport.OutboundContext) {
}

func (h *Outbound) Close() error {
	return nil
}

func (o *Outbound) NewConnection(ctx context.Context, conn *session.Conn) error {
	conn.Conn.Close()
	return nil
}

func (o *Outbound) NewPacketConnection(ctx context.Context, packetConn *session.PacketConn) error {
	packetConn.Conn.Close()
	return nil
}
