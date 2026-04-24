package bufio

import (
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func (c *bidirectionalNATPacketConn) CreatePacketReadWaiter() (N.PacketReadWaiter, bool) {
	waiter, created := CreatePacketReadWaiter(c.NetPacketConn)
	if !created {
		return nil, false
	}
	return &waitBidirectionalNATPacketConn{c, waiter}, true
}

func (c *bidirectionalNATPacketConn) CreatePacketBatchReadWaiter() (N.PacketBatchReadWaiter, bool) {
	waiter, created := CreatePacketBatchReadWaiter(c.NetPacketConn)
	if !created {
		return nil, false
	}
	return &batchWaitBidirectionalNATPacketConn{c, waiter}, true
}

func (c *unidirectionalNATPacketConn) CreateConnectedPacketBatchReadWaiter() (N.ConnectedPacketBatchReadWaiter, bool) {
	return CreateConnectedPacketBatchReadWaiter(c.NetPacketConn)
}

func (c *bidirectionalNATPacketConn) CreateConnectedPacketBatchReadWaiter() (N.ConnectedPacketBatchReadWaiter, bool) {
	waiter, created := CreateConnectedPacketBatchReadWaiter(c.NetPacketConn)
	if !created {
		return nil, false
	}
	return &connectedBatchWaitBidirectionalNATPacketConn{c, waiter}, true
}

func (c *destinationNATPacketConn) CreateConnectedPacketBatchReadWaiter() (N.ConnectedPacketBatchReadWaiter, bool) {
	waiter, created := CreateConnectedPacketBatchReadWaiter(c.NetPacketConn)
	if !created {
		return nil, false
	}
	return &connectedBatchWaitDestinationNATPacketConn{c, waiter}, true
}

type waitBidirectionalNATPacketConn struct {
	*bidirectionalNATPacketConn
	readWaiter N.PacketReadWaiter
}

func (c *waitBidirectionalNATPacketConn) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	return c.readWaiter.InitializeReadWaiter(options)
}

func (c *waitBidirectionalNATPacketConn) WaitReadPacket() (buffer *buf.Buffer, destination M.Socksaddr, err error) {
	buffer, destination, err = c.readWaiter.WaitReadPacket()
	if err != nil {
		return
	}
	if socksaddrWithoutPort(destination) == c.origin {
		destination = M.Socksaddr{
			Addr: c.destination.Addr,
			Fqdn: c.destination.Fqdn,
			Port: destination.Port,
		}
	}
	return
}

type connectedBatchWaitBidirectionalNATPacketConn struct {
	*bidirectionalNATPacketConn
	readWaiter N.ConnectedPacketBatchReadWaiter
}

func (c *connectedBatchWaitBidirectionalNATPacketConn) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	return c.readWaiter.InitializeReadWaiter(options)
}

func (c *connectedBatchWaitBidirectionalNATPacketConn) WaitReadConnectedPackets() (buffers []*buf.Buffer, destination M.Socksaddr, err error) {
	buffers, destination, err = c.readWaiter.WaitReadConnectedPackets()
	if err != nil {
		return
	}
	if socksaddrWithoutPort(destination) == c.origin {
		destination = M.Socksaddr{
			Addr: c.destination.Addr,
			Fqdn: c.destination.Fqdn,
			Port: destination.Port,
		}
	}
	return
}

type connectedBatchWaitDestinationNATPacketConn struct {
	*destinationNATPacketConn
	readWaiter N.ConnectedPacketBatchReadWaiter
}

func (c *connectedBatchWaitDestinationNATPacketConn) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	return c.readWaiter.InitializeReadWaiter(options)
}

func (c *connectedBatchWaitDestinationNATPacketConn) WaitReadConnectedPackets() (buffers []*buf.Buffer, destination M.Socksaddr, err error) {
	buffers, destination, err = c.readWaiter.WaitReadConnectedPackets()
	if err != nil {
		return
	}
	if destination == c.origin {
		destination = c.destination
	}
	return
}

type batchWaitBidirectionalNATPacketConn struct {
	*bidirectionalNATPacketConn
	readWaiter N.PacketBatchReadWaiter
}

func (c *batchWaitBidirectionalNATPacketConn) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	return c.readWaiter.InitializeReadWaiter(options)
}

func (c *batchWaitBidirectionalNATPacketConn) WaitReadPackets() (buffers []*buf.Buffer, destinations []M.Socksaddr, err error) {
	buffers, destinations, err = c.readWaiter.WaitReadPackets()
	if err != nil {
		return
	}
	for index, destination := range destinations {
		if socksaddrWithoutPort(destination) == c.origin {
			destinations[index] = M.Socksaddr{
				Addr: c.destination.Addr,
				Fqdn: c.destination.Fqdn,
				Port: destination.Port,
			}
		}
	}
	return
}
