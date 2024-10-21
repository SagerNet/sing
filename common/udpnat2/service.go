package udpnat

import (
	"context"
	"net/netip"
	"time"

	"github.com/sagernet/sing/common"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/pipe"
	"github.com/sagernet/sing/contrab/freelru"
	"github.com/sagernet/sing/contrab/maphash"
)

type Service struct {
	nat     *freelru.LRU[netip.AddrPort, *natConn]
	handler N.UDPConnectionHandlerEx
	prepare PrepareFunc
	metrics Metrics
}

type PrepareFunc func(source M.Socksaddr, destination M.Socksaddr, userData any) (bool, context.Context, N.PacketWriter, N.CloseHandlerFunc)

type Metrics struct {
	Creates uint64
	Rejects uint64
	Inputs  uint64
	Drops   uint64
}

func New(handler N.UDPConnectionHandlerEx, prepare PrepareFunc, timeout time.Duration) *Service {
	nat := common.Must1(freelru.New[netip.AddrPort, *natConn](1024, maphash.NewHasher[netip.AddrPort]().Hash32))
	nat.SetLifetime(timeout)
	nat.SetHealthCheck(func(port netip.AddrPort, conn *natConn) bool {
		select {
		case <-conn.doneChan:
			return false
		default:
			return true
		}
	})
	nat.SetOnEvict(func(_ netip.AddrPort, conn *natConn) {
		conn.Close()
	})
	return &Service{
		nat:     nat,
		handler: handler,
		prepare: prepare,
	}
}

func (s *Service) NewPacket(bufferSlices [][]byte, source M.Socksaddr, destination M.Socksaddr, userData any) {
	conn, loaded := s.nat.Get(source.AddrPort())
	if !loaded {
		ok, ctx, writer, onClose := s.prepare(source, destination, userData)
		if !ok {
			s.metrics.Rejects++
			return
		}
		conn = &natConn{
			writer:       writer,
			localAddr:    source,
			packetChan:   make(chan *Packet, 64),
			doneChan:     make(chan struct{}),
			readDeadline: pipe.MakeDeadline(),
		}
		s.nat.Add(source.AddrPort(), conn)
		s.handler.NewPacketConnectionEx(ctx, conn, source, destination, onClose)
		s.metrics.Creates++
	}
	packet := NewPacket()
	buffer := conn.readWaitOptions.NewPacketBuffer()
	for _, bufferSlice := range bufferSlices {
		buffer.Write(bufferSlice)
	}
	*packet = Packet{
		Buffer:      buffer,
		Destination: destination,
	}
	select {
	case conn.packetChan <- packet:
		s.metrics.Inputs++
	default:
		packet.Buffer.Release()
		PutPacket(packet)
		s.metrics.Drops++
	}
}

func (s *Service) Metrics() Metrics {
	return s.metrics
}
