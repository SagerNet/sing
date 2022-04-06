package system

import (
	"net"
	"net/netip"
)

type TCPHandler interface {
	HandleTCP(conn net.Conn) error
	OnError(err error)
}

type TCPListener struct {
	Listen  netip.AddrPort
	Handler TCPHandler
	*net.TCPListener
}

func NewTCPListener(listen netip.AddrPort, handler TCPHandler) *TCPListener {
	return &TCPListener{
		Listen:  listen,
		Handler: handler,
	}
}

func (l *TCPListener) Start() error {
	tcpListener, err := net.ListenTCP("tcp", net.TCPAddrFromAddrPort(l.Listen))
	if err != nil {
		return err
	}
	l.TCPListener = tcpListener
	go l.loop()
	return nil
}

func (l *TCPListener) Close() error {
	if l == nil || l.TCPListener == nil {
		return nil
	}
	return l.TCPListener.Close()
}

func (l *TCPListener) loop() {
	for {
		tcpConn, err := l.Accept()
		if err != nil {
			l.Close()
			return
		}
		go func() {
			err := l.Handler.HandleTCP(tcpConn)
			if err != nil {
				l.Handler.OnError(err)
			}
		}()
	}
}
