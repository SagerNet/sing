package system

import (
	"net"

	"net/netip"
	"sing/common/buf"
)

type UDPHandler interface {
	HandleUDP(buffer *buf.Buffer, sourceAddr net.Addr) error
	OnError(err error)
}

type UDPListener struct {
	Listen  netip.AddrPort
	Handler UDPHandler
	*net.UDPConn
}

func NewUDPListener(listen netip.AddrPort, handler UDPHandler) *UDPListener {
	return &UDPListener{
		Listen:  listen,
		Handler: handler,
	}
}

func (l *UDPListener) Start() error {
	udpConn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(l.Listen))
	if err != nil {
		return err
	}
	l.UDPConn = udpConn
	go l.loop()
	return nil
}

func (l *UDPListener) Close() error {
	if l == nil || l.UDPConn == nil {
		return nil
	}
	return l.UDPConn.Close()
}

func (l *UDPListener) loop() {
	for {
		buffer := buf.New()
		n, addr, err := l.ReadFromUDP(buffer.Extend(buf.UDPBufferSize))
		if err != nil {
			buffer.Release()
			return
		}
		buffer.Truncate(n)
		go func() {
			err := l.Handler.HandleUDP(buffer, addr)
			if err != nil {
				buffer.Release()
				l.Handler.OnError(err)
			}
		}()
	}
}
