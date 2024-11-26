package udpnat

import (
	"context"
	"net/netip"
	"sync"
	"time"

	"github.com/sagernet/sing/common"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/pipe"
	"github.com/sagernet/sing/contrab/freelru"
	"github.com/sagernet/sing/contrab/maphash"
)

type Service struct {
	cache   freelru.Cache[netip.AddrPort, *natConn]
	handler N.UDPConnectionHandlerEx
	prepare PrepareFunc
	metrics Metrics

	timeout   time.Duration
	closeOnce sync.Once
	doneChan  chan struct{}
}

type PrepareFunc func(source M.Socksaddr, destination M.Socksaddr, userData any) (bool, context.Context, N.PacketWriter, N.CloseHandlerFunc)

type Metrics struct {
	Creates uint64
	Rejects uint64
	Inputs  uint64
	Drops   uint64
}

func New(handler N.UDPConnectionHandlerEx, prepare PrepareFunc, timeout time.Duration, shared bool) *Service {
	if timeout == 0 {
		panic("invalid timeout")
	}
	var cache freelru.Cache[netip.AddrPort, *natConn]
	if !shared {
		cache = common.Must1(freelru.New[netip.AddrPort, *natConn](1024, maphash.NewHasher[netip.AddrPort]().Hash32))
	} else {
		cache = common.Must1(freelru.NewSharded[netip.AddrPort, *natConn](1024, maphash.NewHasher[netip.AddrPort]().Hash32))
	}
	cache.SetLifetime(timeout)
	cache.SetUpdateLifetimeOnGet(true)
	cache.SetHealthCheck(func(port netip.AddrPort, conn *natConn) bool {
		select {
		case <-conn.doneChan:
			return false
		default:
			return true
		}
	})
	cache.SetOnEvict(func(_ netip.AddrPort, conn *natConn) {
		conn.Close()
	})
	return &Service{
		cache:    cache,
		handler:  handler,
		prepare:  prepare,
		timeout:  timeout,
		doneChan: make(chan struct{}),
	}
}

func (s *Service) Start() error {
	ticker := time.NewTicker(s.timeout)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.PurgeExpired()
			case <-s.doneChan:
				s.Purge()
				return
			}
		}
	}()
	return nil
}

func (s *Service) Close() {
	s.closeOnce.Do(func() {
		close(s.doneChan)
	})
}

func (s *Service) NewPacket(bufferSlices [][]byte, source M.Socksaddr, destination M.Socksaddr, userData any) {
	conn, loaded := s.cache.Get(source.AddrPort())
	if !loaded {
		ok, ctx, writer, onClose := s.prepare(source, destination, userData)
		if !ok {
			s.metrics.Rejects++
			return
		}
		conn = &natConn{
			service:      s,
			writer:       writer,
			localAddr:    source,
			packetChan:   make(chan *N.PacketBuffer, 64),
			doneChan:     make(chan struct{}),
			readDeadline: pipe.MakeDeadline(),
		}
		s.cache.Add(source.AddrPort(), conn)
		go s.handler.NewPacketConnectionEx(ctx, conn, source, destination, onClose)
		s.metrics.Creates++
	}
	buffer := conn.readWaitOptions.NewPacketBuffer()
	for _, bufferSlice := range bufferSlices {
		buffer.Write(bufferSlice)
	}
	if conn.handler != nil {
		conn.handler.NewPacketEx(buffer, destination)
		return
	}
	packet := N.NewPacketBuffer()
	*packet = N.PacketBuffer{
		Buffer:      buffer,
		Destination: destination,
	}
	select {
	case conn.packetChan <- packet:
		s.metrics.Inputs++
	default:
		packet.Buffer.Release()
		N.PutPacketBuffer(packet)
		s.metrics.Drops++
	}
}

func (s *Service) Purge() {
	s.cache.Purge()
}

func (s *Service) PurgeExpired() {
	s.cache.PurgeExpired()
}

func (s *Service) Metrics() Metrics {
	return s.metrics
}

func (s *Service) ResetMetrics() Metrics {
	metrics := s.metrics
	s.metrics = Metrics{}
	return metrics
}
