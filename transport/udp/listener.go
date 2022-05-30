package udp

import (
	"context"
	"net"
	"net/netip"
	"os"
	"sync"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/redir"
)

type Handler interface {
	N.UDPHandler
	E.Handler
}

type Listener struct {
	*net.UDPConn
	handler    Handler
	network    string
	bind       netip.AddrPort
	tproxy     bool
	forceAddr6 bool
	ctx        context.Context
	access     sync.RWMutex
	closed     chan struct{}
	outbound   chan *outboundPacket
}

func (l *Listener) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	n, addr, err := l.ReadFromUDPAddrPort(buffer.FreeBytes())
	if err != nil {
		return M.Socksaddr{}, err
	}
	buffer.Truncate(n)
	return M.SocksaddrFromNetIP(addr), nil
}

func (l *Listener) WriteIsThreadUnsafe() {
}

func (l *Listener) loopBack() {
	for {
		select {
		case packet := <-l.outbound:
			err := l.writePacket(packet.buffer, packet.destination)
			if err != nil && !E.IsClosed(err) {
				l.handler.HandleError(E.New("udp write failed: ", err))
			}
			continue
		case <-l.closed:
		}
		for {
			select {
			case packet := <-l.outbound:
				packet.buffer.Release()
			default:
				return
			}
		}
	}
}

type outboundPacket struct {
	buffer      *buf.Buffer
	destination M.Socksaddr
}

func (l *Listener) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	l.access.RLock()
	defer l.access.RUnlock()

	select {
	case <-l.closed:
		return os.ErrClosed
	default:
	}

	l.outbound <- &outboundPacket{buffer, destination}
	return nil
}

func (l *Listener) writePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	defer buffer.Release()
	if destination.Family().IsFqdn() {
		udpAddr, err := net.ResolveUDPAddr("udp", destination.String())
		if err != nil {
			return err
		}
		return common.Error(l.UDPConn.WriteTo(buffer.Bytes(), udpAddr))
	}
	if l.forceAddr6 && destination.Addr.Is4() {
		destination.Addr = netip.AddrFrom16(destination.Addr.As16())
	}
	return common.Error(l.UDPConn.WriteToUDPAddrPort(buffer.Bytes(), destination.AddrPort()))
}

func NewUDPListener(listen netip.AddrPort, handler Handler, options ...Option) *Listener {
	listener := &Listener{
		handler:  handler,
		bind:     listen,
		outbound: make(chan *outboundPacket),
		closed:   make(chan struct{}),
	}
	for _, option := range options {
		option(listener)
	}
	return listener
}

func (l *Listener) Start() error {
	udpConn, err := net.ListenUDP(M.NetworkFromNetAddr("udp", l.bind.Addr()), net.UDPAddrFromAddrPort(l.bind))
	if err != nil {
		return err
	}
	l.forceAddr6 = l.bind.Addr().Is6()

	if l.tproxy {
		fd, err := common.GetFileDescriptor(udpConn)
		if err != nil {
			return err
		}
		err = redir.TProxy(fd, l.bind.Addr().Is6())
		if err != nil {
			return E.Cause(err, "configure tproxy")
		}
		err = redir.TProxyUDP(fd, l.bind.Addr().Is6())
		if err != nil {
			return E.Cause(err, "configure tproxy")
		}
	}

	l.UDPConn = udpConn

	if _, threadUnsafeHandler := common.Cast[N.ThreadUnsafeWriter](l.handler); threadUnsafeHandler {
		go l.loopThreadSafe()
	} else {
		go l.loop()
	}

	go l.loopBack()
	return nil
}

func (l *Listener) Close() error {
	if l == nil || l.UDPConn == nil {
		return nil
	}
	return l.UDPConn.Close()
}

func (l *Listener) loop() {
	defer close(l.closed)

	_buffer := buf.StackNewPacket()
	defer common.KeepAlive(_buffer)
	buffer := common.Dup(_buffer)
	buffer.IncRef()
	defer buffer.DecRef()
	if !l.tproxy {
		for {
			buffer.Reset()
			n, addr, err := l.ReadFromUDPAddrPort(buffer.FreeBytes())
			if err != nil {
				l.handler.HandleError(E.New("udp listener closed: ", err))
				return
			}
			buffer.Truncate(n)
			err = l.handler.NewPacket(context.Background(), l, buffer, M.Metadata{
				Protocol: "udp",
				Source:   M.SocksaddrFromNetIP(addr),
			})
			if err != nil {
				l.handler.HandleError(err)
			}
		}
	} else {
		_oob := make([]byte, 1024)
		defer common.KeepAlive(_oob)
		oob := common.Dup(_oob)
		for {
			buffer.Reset()
			n, oobN, _, addr, err := l.ReadMsgUDPAddrPort(buffer.FreeBytes(), oob)
			if err != nil {
				l.handler.HandleError(E.New("udp listener closed: ", err))
				return
			}
			buffer.Truncate(n)
			destination, err := redir.GetOriginalDestinationFromOOB(oob[:oobN])
			if err != nil {
				l.handler.HandleError(E.Cause(err, "get original destination"))
				continue
			}
			buffer.Resize(buf.ReversedHeader, n)
			err = l.handler.NewPacket(context.Background(), l, buffer, M.Metadata{
				Protocol:    "tproxy",
				Source:      M.SocksaddrFromNetIP(addr),
				Destination: M.SocksaddrFromNetIP(destination),
			})
			if err != nil {
				l.handler.HandleError(err)
			}
		}

	}
}

func (l *Listener) loopThreadSafe() {
	defer close(l.closed)

	if !l.tproxy {
		for {
			buffer := buf.NewPacket()
			n, addr, err := l.ReadFromUDPAddrPort(buffer.FreeBytes())
			if err != nil {
				buffer.Release()
				l.handler.HandleError(E.New("udp listener closed: ", err))
				return
			}
			buffer.Truncate(n)
			err = l.handler.NewPacket(context.Background(), l, buffer, M.Metadata{
				Protocol: "udp",
				Source:   M.SocksaddrFromNetIP(addr),
			})
			if err != nil {
				buffer.Release()
				l.handler.HandleError(err)
			}
		}
	} else {
		_oob := make([]byte, 1024)
		defer common.KeepAlive(_oob)
		oob := common.Dup(_oob)
		for {
			buffer := buf.NewPacket()
			n, oobN, _, addr, err := l.ReadMsgUDPAddrPort(buffer.FreeBytes(), oob)
			if err != nil {
				l.handler.HandleError(E.New("udp listener closed: ", err))
				return
			}
			buffer.Truncate(n)
			destination, err := redir.GetOriginalDestinationFromOOB(oob[:oobN])
			if err != nil {
				buffer.Release()
				l.handler.HandleError(E.Cause(err, "get original destination"))
				continue
			}
			buffer.Resize(buf.ReversedHeader, n)
			err = l.handler.NewPacket(context.Background(), l, buffer, M.Metadata{
				Protocol:    "tproxy",
				Source:      M.SocksaddrFromNetIP(addr),
				Destination: M.SocksaddrFromNetIP(destination),
			})
			if err != nil {
				buffer.Release()
				l.handler.HandleError(err)
			}
		}

	}
}
