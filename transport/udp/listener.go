package udp

import (
	"context"
	"net"
	"net/netip"
	"runtime"

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
	handler Handler
	network string
	bind    netip.AddrPort
	tproxy  bool
}

func (l *Listener) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	n, addr, err := l.ReadFromUDPAddrPort(buffer.FreeBytes())
	if err != nil {
		return M.Socksaddr{}, err
	}
	buffer.Truncate(n)
	return M.SocksaddrFromNetIP(addr), nil
}

func (l *Listener) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	if destination.Family().IsFqdn() {
		udpAddr, err := net.ResolveUDPAddr("udp", destination.String())
		if err != nil {
			return err
		}
		return common.Error(l.UDPConn.WriteTo(buffer.Bytes(), udpAddr))
	}
	return common.Error(l.UDPConn.WriteToUDP(buffer.Bytes(), destination.UDPAddr()))
}

func NewUDPListener(listen netip.AddrPort, handler Handler, options ...Option) *Listener {
	listener := &Listener{
		handler: handler,
		bind:    listen,
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
	go l.loop()
	return nil
}

func (l *Listener) Close() error {
	if l == nil || l.UDPConn == nil {
		return nil
	}
	return l.UDPConn.Close()
}

func (l *Listener) loop() {
	_buffer := buf.StackNewMax()
	defer runtime.KeepAlive(_buffer)
	buffer := common.Dup(_buffer)
	data := buffer.Cut(buf.ReversedHeader, buf.ReversedHeader).Slice()
	if !l.tproxy {
		for {
			n, addr, err := l.ReadFromUDPAddrPort(data)
			if err != nil {
				l.handler.HandleError(E.New("udp listener closed: ", err))
				return
			}
			buffer.Resize(buf.ReversedHeader, n)
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
		defer runtime.KeepAlive(_oob)
		oob := common.Dup(_oob)
		for {
			n, oobN, _, addr, err := l.ReadMsgUDPAddrPort(data, oob)
			if err != nil {
				l.handler.HandleError(E.New("udp listener closed: ", err))
				return
			}
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
