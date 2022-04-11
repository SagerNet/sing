package tcp

import (
	"net"
	"net/netip"

	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/redir"
)

type Handler interface {
	M.TCPConnectionHandler
	E.Handler
}

type Listener struct {
	bind    netip.AddrPort
	handler Handler
	trans   redir.TransproxyMode
	lAddr   *net.TCPAddr
	*net.TCPListener
}

func NewTCPListener(listen netip.AddrPort, handler Handler, options ...Option) *Listener {
	listener := &Listener{
		bind:    listen,
		handler: handler,
	}
	for _, option := range options {
		option(listener)
	}
	return listener
}

func (l *Listener) Start() error {
	network := "tcp"
	if l.bind.Addr() == netip.IPv4Unspecified() {
		network = "tcp4"
	}
	tcpListener, err := net.ListenTCP(network, net.TCPAddrFromAddrPort(l.bind))
	if err != nil {
		return err
	}
	if l.trans == redir.ModeTProxy {
		l.lAddr = tcpListener.Addr().(*net.TCPAddr)
		fd, err := common.GetFileDescriptor(tcpListener)
		if err != nil {
			return err
		}
		err = redir.TProxy(fd, l.bind.Addr().Is6())
		if err != nil {
			return E.Cause(err, "configure tproxy")
		}
	}
	l.TCPListener = tcpListener
	go l.loop()
	return nil
}

func (l *Listener) Close() error {
	if l == nil || l.TCPListener == nil {
		return nil
	}
	return l.TCPListener.Close()
}

func (l *Listener) loop() {
	for {
		tcpConn, err := l.Accept()
		if err != nil {
			l.Close()
			return
		}
		var metadata M.Metadata
		switch l.trans {
		case redir.ModeRedirect:
			metadata.Destination, _ = redir.GetOriginalDestination(tcpConn)
		case redir.ModeTProxy:
			lAddr := tcpConn.LocalAddr().(*net.TCPAddr)
			rAddr := tcpConn.RemoteAddr().(*net.TCPAddr)

			if lAddr.Port != l.lAddr.Port || !lAddr.IP.Equal(rAddr.IP) && !lAddr.IP.IsLoopback() && !lAddr.IP.IsPrivate() {
				metadata.Destination = M.AddrPortFromNetAddr(lAddr)
			}
		}
		go func() {
			err := l.handler.NewConnection(tcpConn, metadata)
			if err != nil {
				l.handler.HandleError(err)
			}
		}()
	}
}
