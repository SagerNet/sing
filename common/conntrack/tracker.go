package conntrack

import (
	"io"
	"net"
	"sync"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/bufio"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/x/list"
)

type Tracker struct {
	access      sync.Mutex
	connections list.List[io.Closer]
}

func (m *Tracker) Track(conn io.Closer) *Registration {
	m.access.Lock()
	element := m.connections.PushBack(conn)
	m.access.Unlock()
	return &Registration{m, element}
}

func (m *Tracker) TrackConn(conn net.Conn) net.Conn {
	registration := m.Track(conn)
	return &trackConn{conn, registration}
}

func (m *Tracker) TrackPacketConn(conn net.PacketConn) N.NetPacketConn {
	registration := m.Track(conn)
	return &trackPacketConn{bufio.NewPacketConn(conn), registration}
}

func (m *Tracker) Reset() {
	m.access.Lock()
	defer m.access.Unlock()
	for element := m.connections.Front(); element != nil; element = element.Next() {
		common.Close(element.Value)
	}
	m.connections = list.List[io.Closer]{}
}

type Registration struct {
	manager *Tracker
	element *list.Element[io.Closer]
}

func (t *Registration) Leave() {
	t.manager.access.Lock()
	defer t.manager.access.Unlock()
	t.manager.connections.Remove(t.element)
}

type trackConn struct {
	net.Conn
	registration *Registration
}

func (t *trackConn) Close() error {
	t.registration.Leave()
	return t.Conn.Close()
}

func (t *trackConn) WriteTo(w io.Writer) (n int64, err error) {
	return bufio.Copy(w, t.Conn)
}

func (t *trackConn) ReadFrom(r io.Reader) (n int64, err error) {
	return bufio.Copy(t.Conn, r)
}

func (t *trackConn) Upstream() any {
	return t.Conn
}

func (t *trackConn) ReaderReplaceable() bool {
	return true
}

func (t *trackConn) WriterReplaceable() bool {
	return true
}

type trackPacketConn struct {
	N.NetPacketConn
	registration *Registration
}

func (t *trackPacketConn) Close() error {
	t.registration.Leave()
	return t.NetPacketConn.Close()
}

func (t *trackPacketConn) Upstream() any {
	return t.NetPacketConn
}
