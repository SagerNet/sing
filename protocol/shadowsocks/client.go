package shadowsocks

import (
	"context"
	"io"
	"net"
	"strconv"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/common/socksaddr"
	"github.com/sagernet/sing/protocol/socks"
)

var (
	ErrBadKey          = exceptions.New("bad key")
	ErrMissingPassword = exceptions.New("password not specified")
)

type ClientConfig struct {
	Server     string `json:"server"`
	ServerPort uint16 `json:"server_port"`
	Method     string `json:"method"`
	Password   []byte `json:"password"`
	Key        []byte `json:"key"`
}

type Client struct {
	dialer *net.Dialer
	cipher Cipher
	server string
	key    []byte
}

func NewClient(dialer *net.Dialer, config *ClientConfig) (*Client, error) {
	if config.Server == "" {
		return nil, exceptions.New("missing server address")
	}
	if config.ServerPort == 0 {
		return nil, exceptions.New("missing server port")
	}
	if config.Method == "" {
		return nil, exceptions.New("missing server method")
	}

	cipher, err := CreateCipher(config.Method)
	if err != nil {
		return nil, err
	}
	client := &Client{
		dialer: dialer,
		cipher: cipher,
		server: net.JoinHostPort(config.Server, strconv.Itoa(int(config.ServerPort))),
	}
	if keyLen := len(config.Key); keyLen > 0 {
		if keyLen == cipher.KeySize() {
			client.key = config.Key
		} else {
			return nil, ErrBadKey
		}
	} else if len(config.Password) > 0 {
		client.key = Key(config.Password, cipher.KeySize())
	} else {
		return nil, ErrMissingPassword
	}
	return client, nil
}

func (c *Client) DialContextTCP(ctx context.Context, addr socksaddr.Addr, port uint16) (net.Conn, error) {
	conn, err := c.dialer.DialContext(ctx, "tcp", c.server)
	if err != nil {
		return nil, exceptions.Cause(err, "connect to server")
	}
	return c.DialConn(conn, addr, port), nil
}

func (c *Client) DialConn(conn net.Conn, addr socksaddr.Addr, port uint16) net.Conn {
	header := buf.New()
	header.WriteRandom(c.cipher.SaltSize())
	writer := &buf.BufferedWriter{
		Writer: conn,
		Buffer: header,
	}
	protocolWriter := c.cipher.CreateWriter(c.key, header.Bytes(), writer)
	requestBuffer := buf.New()
	contentWriter := &buf.BufferedWriter{
		Writer: protocolWriter,
		Buffer: requestBuffer,
	}
	common.Must(AddressSerializer.WriteAddressAndPort(contentWriter, addr, port))
	return &shadowsocksConn{
		Client: c,
		Conn:   conn,
		Writer: &common.FlushOnceWriter{Writer: contentWriter},
	}
}

type shadowsocksConn struct {
	*Client
	net.Conn
	io.Writer
	reader io.Reader
}

func (c *shadowsocksConn) Read(p []byte) (n int, err error) {
	if c.reader == nil {
		buffer := buf.Or(p, c.cipher.SaltSize())
		defer buffer.Release()
		_, err = buffer.ReadFullFrom(c.Conn, c.cipher.SaltSize())
		if err != nil {
			return
		}
		c.reader = c.cipher.CreateReader(c.key, buffer.Bytes(), c.Conn)
	}
	return c.reader.Read(p)
}

func (c *shadowsocksConn) WriteTo(w io.Writer) (n int64, err error) {
	if c.reader == nil {
		buffer := buf.NewSize(c.cipher.SaltSize())
		defer buffer.Release()
		_, err = buffer.ReadFullFrom(c.Conn, c.cipher.SaltSize())
		if err != nil {
			return
		}
		c.reader = c.cipher.CreateReader(c.key, buffer.Bytes(), c.Conn)
	}
	return c.reader.(io.WriterTo).WriteTo(w)
}

func (c *shadowsocksConn) Write(p []byte) (n int, err error) {
	return c.Writer.Write(p)
}

func (c *shadowsocksConn) ReadFrom(r io.Reader) (n int64, err error) {
	return rw.ReadFromVar(&c.Writer, r)
}

func (c *Client) DialContextUDP(ctx context.Context) socks.PacketConn {
	conn, err := c.dialer.DialContext(ctx, "udp", c.server)
	if err != nil {
		return nil
	}
	return &shadowsocksPacketConn{c, conn}
}

type shadowsocksPacketConn struct {
	*Client
	net.Conn
}

func (c *shadowsocksPacketConn) WritePacket(buffer *buf.Buffer, addr socksaddr.Addr, port uint16) error {
	defer buffer.Release()
	header := buf.New()
	header.WriteRandom(c.cipher.SaltSize())
	common.Must(AddressSerializer.WriteAddressAndPort(header, addr, port))
	buffer = buffer.WriteBufferAtFirst(header)
	err := c.cipher.EncodePacket(c.key, buffer)
	if err != nil {
		return err
	}
	return common.Error(c.Conn.Write(buffer.Bytes()))
}

func (c *shadowsocksPacketConn) ReadPacket(buffer *buf.Buffer) (socksaddr.Addr, uint16, error) {
	n, err := c.Read(buffer.FreeBytes())
	if err != nil {
		return nil, 0, err
	}
	buffer.Truncate(n)
	err = c.cipher.DecodePacket(c.key, buffer)
	if err != nil {
		return nil, 0, err
	}
	return AddressSerializer.ReadAddressAndPort(buffer)
}
