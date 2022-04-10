package udpnat

import (
	"io"
	"net"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/gsync"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/redir"
	"github.com/sagernet/sing/protocol/socks"
)

type Handler interface {
	socks.UDPConnectionHandler
	E.Handler
}

type Server struct {
	udpNat  gsync.Map[string, *packetConn]
	handler Handler
}

func NewServer(handler Handler) *Server {
	return &Server{handler: handler}
}

func (s *Server) HandleUDP(buffer *buf.Buffer, metadata M.Metadata) error {
	conn, loaded := s.udpNat.LoadOrStore(metadata.Source.String(), func() *packetConn {
		return &packetConn{source: metadata.Source.UDPAddr(), in: make(chan *udpPacket)}
	})
	if !loaded {
		go func() {
			err := s.handler.NewPacketConnection(conn, metadata)
			if err != nil {
				s.handler.HandleError(err)
			}
		}()
	}
	conn.in <- &udpPacket{
		buffer:      buffer,
		destination: metadata.Destination,
	}
	return nil
}

func (s *Server) OnError(err error) {
	s.handler.HandleError(err)
}

func (s *Server) Close() error {
	s.udpNat.Range(func(key string, conn *packetConn) bool {
		conn.Close()
		return true
	})
	s.udpNat = gsync.Map[string, *packetConn]{}
	return nil
}

type packetConn struct {
	socks.PacketConnStub
	source *net.UDPAddr
	in     chan *udpPacket
}

type udpPacket struct {
	buffer      *buf.Buffer
	destination *M.AddrPort
}

func (c *packetConn) LocalAddr() net.Addr {
	return c.source
}

func (c *packetConn) Close() error {
	select {
	case <-c.in:
		return io.ErrClosedPipe
	default:
		close(c.in)
	}
	return nil
}

func (c *packetConn) ReadPacket(buffer *buf.Buffer) (*M.AddrPort, error) {
	select {
	case packet, ok := <-c.in:
		if !ok {
			return nil, io.ErrClosedPipe
		}
		defer packet.buffer.Release()
		if buffer.FreeLen() < packet.buffer.Len() {
			return nil, io.ErrShortBuffer
		}
		return packet.destination, common.Error(buffer.Write(packet.buffer.Bytes()))
	}
}

func (c *packetConn) WritePacket(buffer *buf.Buffer, destination *M.AddrPort) error {
	udpConn, err := redir.DialUDP("udp", destination.UDPAddr(), c.source)
	if err != nil {
		return E.Cause(err, "tproxy udp write back")
	}
	defer udpConn.Close()
	return common.Error(udpConn.Write(buffer.Bytes()))
}
