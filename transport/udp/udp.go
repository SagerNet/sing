package udp

import (
	"net"
	"net/netip"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/redir"
)

type Handler interface {
	M.UDPHandler
	E.Handler
}

type Listener struct {
	*net.UDPConn
	handler Handler
	network string
	bind    netip.AddrPort
	tproxy  bool
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
	if !l.tproxy {
		for {
			buffer := buf.New()
			n, addr, err := l.ReadFromUDP(buffer.Extend(buf.UDPBufferSize))
			if err != nil {
				buffer.Release()
				l.handler.HandleError(err)
				return
			}
			buffer.Truncate(n)
			err = l.handler.NewPacket(buffer, M.Metadata{
				Source: M.AddrPortFromNetAddr(addr),
			})
			if err != nil {
				buffer.Release()
				l.handler.HandleError(err)
			}
		}
	} else {
		oob := make([]byte, 1024)
		for {
			buffer := buf.New()
			n, oobN, _, addr, err := l.ReadMsgUDPAddrPort(buffer.FreeBytes(), oob)
			if err != nil {
				buffer.Release()
				l.handler.HandleError(err)
				return
			}
			destination, err := redir.GetOriginalDestinationFromOOB(oob[:oobN])
			if err != nil {
				l.handler.HandleError(E.Cause(err, "get original destination"))
				return
			}
			buffer.Truncate(n)
			err = l.handler.NewPacket(buffer, M.Metadata{
				Source:      M.AddrPortFromAddrPort(addr),
				Destination: destination,
			})
			if err != nil {
				buffer.Release()
				l.handler.HandleError(err)
			}
		}

	}
}
