package udp

import (
	"net"
	"net/netip"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/redir"
	"github.com/sagernet/sing/protocol/socks"
)

type Handler interface {
	socks.UDPHandler
	E.Handler
}

type Listener struct {
	*net.UDPConn
	handler Handler
	network string
	bind    netip.AddrPort
	tproxy  bool
}

func (l *Listener) ReadPacket(buffer *buf.Buffer) (*M.AddrPort, error) {
	n, addr, err := l.ReadFromUDP(buffer.FreeBytes())
	if err != nil {
		return nil, err
	}
	buffer.Truncate(n)
	return M.AddrPortFromNetAddr(addr), nil
}

func (l *Listener) WritePacket(buffer *buf.Buffer, destination *M.AddrPort) error {
	return common.Error(l.UDPConn.WriteTo(buffer.Bytes(), destination.UDPAddr()))
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
	buffer := common.Dup(_buffer)
	data := buffer.Cut(buf.ReversedHeader, buf.ReversedHeader).Slice()
	if !l.tproxy {
		for {
			n, addr, err := l.ReadFromUDP(data)
			if err != nil {
				l.handler.HandleError(err)
				return
			}
			buffer.Resize(buf.ReversedHeader, n)
			err = l.handler.NewPacket(l, buffer, M.Metadata{
				Protocol: "udp",
				Source:   M.AddrPortFromNetAddr(addr),
			})
			if err != nil {
				l.handler.HandleError(err)
			}
		}
	} else {
		_oob := make([]byte, 1024)
		oob := common.Dup(_oob)
		for {
			n, oobN, _, addr, err := l.ReadMsgUDPAddrPort(data, oob)
			if err != nil {
				l.handler.HandleError(err)
				return
			}
			destination, err := redir.GetOriginalDestinationFromOOB(oob[:oobN])
			if err != nil {
				l.handler.HandleError(E.Cause(err, "get original destination"))
				return
			}
			buffer.Resize(buf.ReversedHeader, n)
			err = l.handler.NewPacket(l, buffer, M.Metadata{
				Protocol:    "tproxy",
				Source:      M.AddrPortFromAddrPort(addr),
				Destination: destination,
			})
			if err != nil {
				l.handler.HandleError(err)
			}
		}

	}
}
