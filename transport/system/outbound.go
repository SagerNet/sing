package system

import (
	"context"
	"net"
	"sing/transport"
	"syscall"

	"sing/common/rw"
	"sing/common/session"
)

var _ transport.Outbound = (*Outbound)(nil)

type Outbound struct{}

func (h *Outbound) Init(*transport.OutboundContext) {
}

func (h *Outbound) Close() error {
	return nil
}

func (h *Outbound) NewConnection(ctx context.Context, conn *session.Conn) error {
	dialer := net.Dialer{
		Control: func(network, address string, c syscall.RawConn) error {
			return ControlRaw(c)
		},
	}
	outConn, err := dialer.DialContext(ctx, "tcp", conn.Context.DestinationNetAddr())
	if err != nil {
		return err
	}
	connElement := session.AddConnection(outConn)
	defer session.RemoveConnection(connElement)
	return rw.CopyConn(ctx, conn.Conn, outConn)
}

func (h *Outbound) NewPacketConnection(ctx context.Context, packetConn *session.PacketConn) error {
	dialer := net.Dialer{
		Control: func(network, address string, c syscall.RawConn) error {
			return ControlRaw(c)
		},
	}
	outConn, err := dialer.DialContext(ctx, "udp", packetConn.Context.DestinationNetAddr())
	if err != nil {
		return err
	}
	connElement := session.AddConnection(outConn)
	defer session.RemoveConnection(connElement)
	return rw.CopyPacketConn(ctx, packetConn.Conn, outConn.(net.PacketConn))
}
