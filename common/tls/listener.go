package tls

import (
	"net"
)

type Listener struct {
	net.Listener
	config ServerConfig
}

func NewListener(inner net.Listener, config ServerConfig) net.Listener {
	l := new(Listener)
	l.Listener = inner
	l.config = config
	return l
}

func (l *Listener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return l.config.Server(conn)
}
