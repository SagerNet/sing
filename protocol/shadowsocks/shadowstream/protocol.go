package shadowstream

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/md5"
	"crypto/rc4"
	"io"
	"net"
	"os"
	"runtime"

	"github.com/dgryski/go-camellia"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/protocol/shadowsocks"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead"
	"golang.org/x/crypto/blowfish"
	"golang.org/x/crypto/cast5"
	"golang.org/x/crypto/chacha20"
)

var List = []string{
	"aes-128-ctr",
	"aes-192-ctr",
	"aes-256-ctr",
	"aes-128-cfb",
	"aes-192-cfb",
	"aes-256-cfb",
	"camellia-128-cfb",
	"camellia-192-cfb",
	"camellia-256-cfb",
	"bf-cfb",
	"cast5-cfb",
	"des-cfb",
	"rc4",
	"rc4-md5",
	"chacha20",
	"chacha20-ietf",
	"xchacha20",
}

type Method struct {
	name               string
	keyLength          int
	saltLength         int
	encryptConstructor func(key []byte, salt []byte) (cipher.Stream, error)
	decryptConstructor func(key []byte, salt []byte) (cipher.Stream, error)
	key                []byte
	secureRNG          io.Reader
}

func New(method string, key []byte, password []byte, secureRNG io.Reader) (shadowsocks.Method, error) {
	m := &Method{
		name:      method,
		secureRNG: secureRNG,
	}
	switch method {
	case "aes-128-ctr":
		m.keyLength = 16
		m.saltLength = aes.BlockSize
		m.encryptConstructor = blockStream(aes.NewCipher, cipher.NewCTR)
		m.decryptConstructor = blockStream(aes.NewCipher, cipher.NewCTR)
	case "aes-192-ctr":
		m.keyLength = 24
		m.saltLength = aes.BlockSize
		m.encryptConstructor = blockStream(aes.NewCipher, cipher.NewCTR)
		m.decryptConstructor = blockStream(aes.NewCipher, cipher.NewCTR)
	case "aes-256-ctr":
		m.keyLength = 32
		m.saltLength = aes.BlockSize
		m.encryptConstructor = blockStream(aes.NewCipher, cipher.NewCTR)
		m.decryptConstructor = blockStream(aes.NewCipher, cipher.NewCTR)
	case "aes-128-cfb":
		m.keyLength = 16
		m.saltLength = aes.BlockSize
		m.encryptConstructor = blockStream(aes.NewCipher, cipher.NewCFBEncrypter)
		m.decryptConstructor = blockStream(aes.NewCipher, cipher.NewCFBDecrypter)
	case "aes-192-cfb":
		m.keyLength = 24
		m.saltLength = aes.BlockSize
		m.encryptConstructor = blockStream(aes.NewCipher, cipher.NewCFBEncrypter)
		m.decryptConstructor = blockStream(aes.NewCipher, cipher.NewCFBDecrypter)
	case "aes-256-cfb":
		m.keyLength = 32
		m.saltLength = aes.BlockSize
		m.encryptConstructor = blockStream(aes.NewCipher, cipher.NewCFBEncrypter)
		m.decryptConstructor = blockStream(aes.NewCipher, cipher.NewCFBDecrypter)
	case "camellia-128-cfb":
		m.keyLength = 16
		m.saltLength = camellia.BlockSize
		m.encryptConstructor = blockStream(camellia.New, cipher.NewCFBEncrypter)
		m.decryptConstructor = blockStream(camellia.New, cipher.NewCFBDecrypter)
	case "camellia-192-cfb":
		m.keyLength = 24
		m.saltLength = camellia.BlockSize
		m.encryptConstructor = blockStream(camellia.New, cipher.NewCFBEncrypter)
		m.decryptConstructor = blockStream(camellia.New, cipher.NewCFBDecrypter)
	case "camellia-256-cfb":
		m.keyLength = 32
		m.saltLength = camellia.BlockSize
		m.encryptConstructor = blockStream(camellia.New, cipher.NewCFBEncrypter)
		m.decryptConstructor = blockStream(camellia.New, cipher.NewCFBDecrypter)
	case "bf-cfb":
		m.keyLength = 16
		m.saltLength = blowfish.BlockSize
		m.encryptConstructor = blockStream(func(key []byte) (cipher.Block, error) { return blowfish.NewCipher(key) }, cipher.NewCFBEncrypter)
		m.decryptConstructor = blockStream(func(key []byte) (cipher.Block, error) { return blowfish.NewCipher(key) }, cipher.NewCFBDecrypter)
	case "cast5-cfb":
		m.keyLength = 16
		m.saltLength = cast5.BlockSize
		m.encryptConstructor = blockStream(func(key []byte) (cipher.Block, error) { return cast5.NewCipher(key) }, cipher.NewCFBEncrypter)
		m.decryptConstructor = blockStream(func(key []byte) (cipher.Block, error) { return cast5.NewCipher(key) }, cipher.NewCFBDecrypter)
	case "des-cfb":
		m.keyLength = 8
		m.saltLength = des.BlockSize
		m.encryptConstructor = blockStream(des.NewCipher, cipher.NewCFBEncrypter)
		m.decryptConstructor = blockStream(des.NewCipher, cipher.NewCFBDecrypter)
	case "rc4":
		m.keyLength = 16
		m.saltLength = 0
		m.encryptConstructor = func(key []byte, salt []byte) (cipher.Stream, error) {
			return rc4.NewCipher(key)
		}
		m.decryptConstructor = func(key []byte, salt []byte) (cipher.Stream, error) {
			return rc4.NewCipher(key)
		}
	case "rc4-md5":
		m.keyLength = 16
		m.saltLength = 0
		m.encryptConstructor = func(key []byte, salt []byte) (cipher.Stream, error) {
			h := md5.New()
			h.Write(key)
			h.Write(salt)
			return rc4.NewCipher(h.Sum(nil))
		}
		m.decryptConstructor = func(key []byte, salt []byte) (cipher.Stream, error) {
			h := md5.New()
			h.Write(key)
			h.Write(salt)
			return rc4.NewCipher(h.Sum(nil))
		}
	case "chacha20", "chacha20-ietf":
		m.keyLength = chacha20.KeySize
		m.saltLength = chacha20.NonceSize
		m.encryptConstructor = func(key []byte, salt []byte) (cipher.Stream, error) {
			return chacha20.NewUnauthenticatedCipher(key, salt)
		}
		m.decryptConstructor = func(key []byte, salt []byte) (cipher.Stream, error) {
			return chacha20.NewUnauthenticatedCipher(key, salt)
		}
	case "xchacha20":
		m.keyLength = chacha20.KeySize
		m.saltLength = chacha20.NonceSizeX
		m.encryptConstructor = func(key []byte, salt []byte) (cipher.Stream, error) {
			return chacha20.NewUnauthenticatedCipher(key, salt)
		}
		m.decryptConstructor = func(key []byte, salt []byte) (cipher.Stream, error) {
			return chacha20.NewUnauthenticatedCipher(key, salt)
		}
	default:
		return nil, os.ErrInvalid
	}
	if len(key) == m.keyLength {
		m.key = key
	} else if len(key) > 0 {
		return nil, shadowaead.ErrBadKey
	} else if len(password) > 0 {
		m.key = shadowsocks.Key(password, m.keyLength)
	} else {
		return nil, shadowaead.ErrMissingPassword
	}
	return m, nil
}

func blockStream(blockCreator func(key []byte) (cipher.Block, error), streamCreator func(block cipher.Block, iv []byte) cipher.Stream) func([]byte, []byte) (cipher.Stream, error) {
	return func(key []byte, iv []byte) (cipher.Stream, error) {
		block, err := blockCreator(key)
		if err != nil {
			return nil, err
		}
		return streamCreator(block, iv), err
	}
}

func (m *Method) Name() string {
	return m.name
}

func (m *Method) KeyLength() int {
	return m.keyLength
}

func (m *Method) DialConn(conn net.Conn, destination M.Socksaddr) (net.Conn, error) {
	shadowsocksConn := &clientConn{
		Conn:        conn,
		method:      m,
		destination: destination,
	}
	return shadowsocksConn, shadowsocksConn.writeRequest(nil)
}

func (m *Method) DialEarlyConn(conn net.Conn, destination M.Socksaddr) net.Conn {
	return &clientConn{
		Conn:        conn,
		method:      m,
		destination: destination,
	}
}

func (m *Method) DialPacketConn(conn net.Conn) N.NetPacketConn {
	return &clientPacketConn{m, conn}
}

type clientConn struct {
	net.Conn

	method      *Method
	destination M.Socksaddr

	readStream  cipher.Stream
	writeStream cipher.Stream
}

func (c *clientConn) writeRequest(payload []byte) error {
	_buffer := buf.Make(c.method.keyLength + M.SocksaddrSerializer.AddrPortLen(c.destination) + len(payload))
	defer runtime.KeepAlive(_buffer)
	buffer := buf.With(common.Dup(_buffer))

	salt := buffer.Extend(c.method.keyLength)
	common.Must1(io.ReadFull(c.method.secureRNG, salt))

	key := shadowaead.Kdf(c.method.key, salt, c.method.keyLength)
	writer, err := c.method.encryptConstructor(c.method.key, salt)
	if err != nil {
		return err
	}
	runtime.KeepAlive(key)

	err = M.SocksaddrSerializer.WriteAddrPort(buffer, c.destination)
	if err != nil {
		return err
	}
	_, err = buffer.Write(payload)
	if err != nil {
		return err
	}

	_, err = c.Conn.Write(buffer.Bytes())
	if err != nil {
		return err
	}

	c.writeStream = writer
	return nil
}

func (c *clientConn) readResponse() error {
	if c.readStream != nil {
		return nil
	}
	_salt := buf.Make(c.method.keyLength)
	defer runtime.KeepAlive(_salt)
	salt := common.Dup(_salt)
	_, err := io.ReadFull(c.Conn, salt)
	if err != nil {
		return err
	}
	key := shadowaead.Kdf(c.method.key, salt, c.method.keyLength)
	defer runtime.KeepAlive(key)
	c.readStream, err = c.method.decryptConstructor(common.Dup(key), salt)
	if err != nil {
		return err
	}
	return nil
}

func (c *clientConn) Read(p []byte) (n int, err error) {
	if err = c.readResponse(); err != nil {
		return
	}
	n, err = c.Conn.Read(p)
	if err != nil {
		return 0, err
	}
	c.readStream.XORKeyStream(p[:n], p[:n])
	return
}

func (c *clientConn) Write(p []byte) (n int, err error) {
	if c.writeStream == nil {
		err = c.writeRequest(p)
		if err == nil {
			n = len(p)
		}
		return
	}

	c.writeStream.XORKeyStream(p, p)
	return c.Conn.Write(p)
}

func (c *clientConn) UpstreamReader() io.Reader {
	return c.Conn
}

func (c *clientConn) ReaderReplaceable() bool {
	return false
}

func (c *clientConn) UpstreamWriter() io.Writer {
	return c.Conn
}

func (c *clientConn) WriterReplaceable() bool {
	return false
}

type clientPacketConn struct {
	*Method
	net.Conn
}

func (c *clientPacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	header := buf.With(buffer.ExtendHeader(c.keyLength + M.SocksaddrSerializer.AddrPortLen(destination)))
	common.Must1(header.ReadFullFrom(c.secureRNG, c.keyLength))
	err := M.SocksaddrSerializer.WriteAddrPort(header, destination)
	if err != nil {
		return err
	}
	stream, err := c.encryptConstructor(c.key, buffer.To(c.keyLength))
	if err != nil {
		return err
	}
	stream.XORKeyStream(buffer.From(c.keyLength), buffer.From(c.keyLength))
	return common.Error(c.Write(buffer.Bytes()))
}

func (c *clientPacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	n, err := c.Read(buffer.FreeBytes())
	if err != nil {
		return M.Socksaddr{}, err
	}
	buffer.Truncate(n)
	stream, err := c.decryptConstructor(c.key, buffer.To(c.keyLength))
	if err != nil {
		return M.Socksaddr{}, err
	}
	stream.XORKeyStream(buffer.From(c.keyLength), buffer.From(c.keyLength))
	buffer.Advance(c.keyLength)
	return M.SocksaddrSerializer.ReadAddrPort(buffer)
}

func (c *clientPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, err = c.Read(p)
	if err != nil {
		return
	}
	stream, err := c.decryptConstructor(c.key, p[:c.keyLength])
	if err != nil {
		return
	}
	buffer := buf.With(p[c.keyLength:n])
	stream.XORKeyStream(buffer.Bytes(), buffer.Bytes())
	destination, err := M.SocksaddrSerializer.ReadAddrPort(buffer)
	if err != nil {
		return
	}
	addr = destination.UDPAddr()
	n = copy(p, buffer.Bytes())
	return
}

func (c *clientPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	destination := M.SocksaddrFromNet(addr)
	_buffer := buf.Make(c.keyLength + M.SocksaddrSerializer.AddrPortLen(destination) + len(p))
	defer runtime.KeepAlive(_buffer)
	buffer := buf.With(common.Dup(_buffer))
	common.Must1(buffer.ReadFullFrom(c.secureRNG, c.keyLength))
	err = M.SocksaddrSerializer.WriteAddrPort(buffer, M.SocksaddrFromNet(addr))
	if err != nil {
		return
	}
	_, err = buffer.Write(p)
	if err != nil {
		return
	}
	stream, err := c.encryptConstructor(c.key, buffer.To(c.keyLength))
	if err != nil {
		return
	}
	stream.XORKeyStream(buffer.From(c.keyLength), buffer.From(c.keyLength))
	_, err = c.Write(buffer.Bytes())
	if err != nil {
		return
	}
	return len(p), nil
}

func (c *clientPacketConn) UpstreamReader() io.Reader {
	return c.Conn
}

func (c *clientPacketConn) ReaderReplaceable() bool {
	return false
}

func (c *clientPacketConn) UpstreamWriter() io.Writer {
	return c.Conn
}

func (c *clientPacketConn) WriterReplaceable() bool {
	return false
}
