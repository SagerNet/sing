package shadowsocks

import (
	"io"
	"net"
	"sync"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/protocol/socks"
)

const MethodNone = "none"

type NoneMethod struct{}

func NewNone() Method {
	return &NoneMethod{}
}

func (m *NoneMethod) Name() string {
	return MethodNone
}

func (m *NoneMethod) KeyLength() int {
	return 0
}

func (m *NoneMethod) DialConn(conn net.Conn, destination *M.AddrPort) (net.Conn, error) {
	shadowsocksConn := &noneConn{
		Conn:        conn,
		handshake:   true,
		destination: destination,
	}
	return shadowsocksConn, shadowsocksConn.clientHandshake()
}

func (m *NoneMethod) DialEarlyConn(conn net.Conn, destination *M.AddrPort) net.Conn {
	return &noneConn{
		Conn:        conn,
		destination: destination,
	}
}

func (m *NoneMethod) DialPacketConn(conn net.Conn) socks.PacketConn {
	return &nonePacketConn{conn}
}

type noneConn struct {
	net.Conn

	access      sync.Mutex
	handshake   bool
	destination *M.AddrPort
}

func (c *noneConn) clientHandshake() error {
	err := socks.AddressSerializer.WriteAddrPort(c.Conn, c.destination)
	if err != nil {
		return err
	}
	c.handshake = true
	return nil
}

func (c *noneConn) Write(b []byte) (n int, err error) {
	if c.handshake {
		goto direct
	}

	c.access.Lock()
	defer c.access.Unlock()

	if c.handshake {
		goto direct
	}

	{
		if len(b) == 0 {
			return 0, c.clientHandshake()
		}

		buffer := buf.New()
		defer buffer.Release()

		err = socks.AddressSerializer.WriteAddrPort(buffer, c.destination)
		if err != nil {
			return
		}

		bufN, _ := buffer.Write(b)
		_, err = c.Conn.Write(buffer.Bytes())
		if err != nil {
			return
		}

		if bufN < len(b) {
			_, err = c.Conn.Write(b[bufN:])
			if err != nil {
				return
			}
		}

		n = len(b)
	}

direct:
	return c.Conn.Write(b)
}

func (c *noneConn) ReadFrom(r io.Reader) (n int64, err error) {
	if !c.handshake {
		panic("missing client handshake")
	}
	return c.Conn.(io.ReaderFrom).ReadFrom(r)
}

func (c *noneConn) WriteTo(w io.Writer) (n int64, err error) {
	return c.Conn.(io.WriterTo).WriteTo(w)
}

func (c *noneConn) RemoteAddr() net.Addr {
	return c.destination.TCPAddr()
}

type nonePacketConn struct {
	net.Conn
}

func (c *nonePacketConn) ReadPacket(buffer *buf.Buffer) (*M.AddrPort, error) {
	_, err := buffer.ReadFrom(c)
	if err != nil {
		return nil, err
	}
	return socks.AddressSerializer.ReadAddrPort(buffer)
}

func (c *nonePacketConn) WritePacket(buffer *buf.Buffer, addrPort *M.AddrPort) error {
	defer buffer.Release()
	header := buf.New()
	err := socks.AddressSerializer.WriteAddrPort(header, addrPort)
	if err != nil {
		header.Release()
		return err
	}
	buffer = buffer.WriteBufferAtFirst(header)
	return common.Error(buffer.WriteTo(c))
}
