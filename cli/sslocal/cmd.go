package sslocal

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sagernet/sing"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/common/socksaddr"
	"github.com/sagernet/sing/common/task"
	"github.com/sagernet/sing/protocol/shadowsocks"
	"github.com/sagernet/sing/protocol/socks"
	"github.com/sagernet/sing/transport/system"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type Flags struct {
	Server     string
	ServerPort uint16
	LocalPort  uint16
	Password   string
	Key        string
	Method     string
}

func MainCmd() *cobra.Command {
	flags := new(Flags)

	cmd := &cobra.Command{
		Use:     "sslocal",
		Short:   "shadowsocks client as socks5 proxy, sing port",
		Version: sing.Version,
		Run: func(cmd *cobra.Command, args []string) {
			Run(flags)
		},
	}

	cmd.Flags().StringVarP(&flags.Server, "server", "s", "", "Set the server’s hostname or IP.")
	cmd.Flags().Uint16VarP(&flags.ServerPort, "server-port", "p", 0, "Set the server’s port number.")
	cmd.Flags().Uint16VarP(&flags.LocalPort, "local-port", "l", 1080, "Set the local port number.")
	cmd.Flags().StringVarP(&flags.Password, "password", "k", "", "Set the password. The server and the client should use the same password.")
	cmd.Flags().StringVar(&flags.Key, "key", "", "Set the key directly. The key should be encoded with URL-safe Base64.")
	cmd.Flags().StringVarP(&flags.Method, "encrypt-method", "m", "", `Set the cipher.

Supported ciphers:

none
aes-128-gcm
aes-192-gcm
aes-256-gcm
chacha20-ietf-poly1305
xchacha20-ietf-poly1305

The default cipher is chacha20-ietf-poly1305.`)
	// cmd.Flags().Uint16VarP(&flags.Timeout, "timeout", "t", 60, "Set the socket timeout in seconds.")
	// cmd.Flags().StringVarP(&flags.ConfigFile, "config", "c", "", "Use a configuration file.")
	// cmd.Flags().Uint16VarP(&flags.MaxFD, "max-open-files", "n", 0, `Specify max number of open files.
	// Only available on Linux.`)
	// cmd.Flags().StringVarP(&flags.Interface, "interface", "i", "", `Send traffic through specific network interface.
	// For example, there are three interfaces in your device, which is lo (127.0.0.1), eth0 (192.168.0.1) and eth1 (192.168.0.2). Meanwhile, you configure ss-local to listen on 0.0.0.0:8388 and bind to eth1. That results the traffic go out through eth1, but not lo nor eth0. This option is useful to control traffic in multi-interface environment.`)
	// cmd.Flags().StringVarP(&flags.LocalAddress, "local-address", "b", "", "Specify the local address to use while this client is making outbound connections to the server.")

	return cmd
}

type LocalClient struct {
	tcpIn      *system.TCPListener
	serverAddr netip.AddrPort
	cipher     shadowsocks.Cipher
	key        []byte
}

func NewLocalClient(flags *Flags) (*LocalClient, error) {
	client := new(LocalClient)
	client.tcpIn = system.NewTCPListener(netip.AddrPortFrom(netip.IPv4Unspecified(), flags.LocalPort), client)

	if flags.Server == "" {
		return nil, exceptions.New("server not specified")
	}

	if addrPort, err := netip.ParseAddrPort(flags.Server); err == nil {
		client.serverAddr = addrPort
	} else if addr, err := netip.ParseAddr(flags.Server); err == nil {
		client.serverAddr = netip.AddrPortFrom(addr, flags.ServerPort)
	} else {
		return nil, err
	}

	cipher, err := shadowsocks.CreateCipher(flags.Method)
	if err != nil {
		return nil, err
	}
	client.cipher = cipher

	if flags.Key != "" {
		key, err := base64.URLEncoding.DecodeString(flags.Key)
		if err != nil {
			return nil, exceptions.Cause(err, "failed to decode base64 key")
		}
		if len(key) != cipher.KeySize() {
			return nil, exceptions.New("key of ", flags.Method, " must be ", cipher.KeySize(), " bytes")
		}
		client.key = key
	} else if flags.Password != "" {
		client.key = shadowsocks.Key([]byte(flags.Password), cipher.KeySize())
	} else {
		return nil, exceptions.New("password not specified")
	}

	return client, nil
}

func Run(flags *Flags) {
	client, err := NewLocalClient(flags)
	if err != nil {
		logrus.Fatal(err)
	}
	client.tcpIn.Start()
	{
		osSignals := make(chan os.Signal, 1)
		signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
		<-osSignals
	}
	client.tcpIn.Close()
}

func (c *LocalClient) HandleTCP(conn net.Conn) error {
	defer conn.Close()

	authRequest, err := socks.ReadAuthRequest(conn)
	if err != nil {
		return err
	}

	if !common.Contains(authRequest.Methods, socks.AuthTypeNotRequired) {
		err = socks.WriteAuthResponse(conn, &socks.AuthResponse{
			Version: authRequest.Version,
			Method:  socks.AuthTypeNoAcceptedMethods,
		})
		if err != nil {
			return err
		}
	}

	err = socks.WriteAuthResponse(conn, &socks.AuthResponse{
		Version: authRequest.Version,
		Method:  socks.AuthTypeNotRequired,
	})
	if err != nil {
		return err
	}

	request, err := socks.ReadRequest(conn)
	if err != nil {
		return err
	}

	ctx := context.Background()

	failure := func() {
		socks.WriteResponse(conn, &socks.Response{
			Version:   request.Version,
			ReplyCode: socks.ReplyCodeFailure,
		})
	}

	switch request.Command {
	case socks.CommandConnect:
		logrus.Info("CONNECT ", request.Addr, ":", request.Port)

		serverConn, dialErr := system.Dial(ctx, "tcp", c.serverAddr.String())
		if dialErr != nil {
			failure()
			return exceptions.Cause(dialErr, "connect to server")
		}
		saltBuffer := buf.New()
		defer saltBuffer.Release()
		if c.cipher.SaltSize() > 0 {
			saltBuffer.WriteRandom(c.cipher.SaltSize())
		}

		serverWriter := &buf.BufferedWriter{
			Writer: serverConn,
			Buffer: saltBuffer,
		}
		writer, _ := c.cipher.CreateWriter(c.key, saltBuffer.Bytes(), serverWriter)

		header := buf.New()
		defer header.Release()

		err = shadowsocks.AddressSerializer.WriteAddressAndPort(header, request.Addr, request.Port)
		if err != nil {
			failure()
			return err
		}

		serverAddr, serverPort := socksaddr.AddressFromNetAddr(serverConn.LocalAddr())
		err = socks.WriteResponse(conn, &socks.Response{
			Version:   request.Version,
			ReplyCode: socks.ReplyCodeSuccess,
			BindAddr:  serverAddr,
			BindPort:  serverPort,
		})
		if err != nil {
			return exceptions.Cause(err, "write response for ", request.Addr, "/", request.Port)
		}

		return task.Run(ctx, func() error {
			// upload
			defer rw.CloseRead(conn)
			defer rw.CloseWrite(serverConn)
			err := conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			if err != nil {
				return err
			}
			_, err = header.ReadFrom(conn)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
				} else {
					return exceptions.Cause(err, "read payload")
				}
			}
			err = conn.SetReadDeadline(time.Time{})
			if err != nil {
				return err
			}
			_, err = writer.Write(header.Bytes())
			if err != nil {
				return err
			}
			err = serverWriter.Flush()
			if err != nil {
				return exceptions.Cause(err, "flush request")
			}
			_, err = io.Copy(writer, conn)
			if err != nil {
				return exceptions.Cause(err, "upload")
			}
			return nil
		}, func() error {
			// download
			defer rw.CloseWrite(conn)
			defer rw.CloseRead(serverConn)

			responseBuffer := buf.New()
			defer responseBuffer.Release()
			_, err := responseBuffer.ReadFullFrom(serverConn, c.cipher.SaltSize())
			if err != nil {
				return exceptions.Cause(err, "read response")
			}
			var salt []byte
			if c.cipher.SaltSize() > 0 {
				salt = responseBuffer.To(c.cipher.SaltSize())
			}

			reader := c.cipher.CreateReader(c.key, salt, serverConn)
			responseBuffer.FullReset()
			_, err = io.CopyBuffer(conn, reader, responseBuffer.FreeBytes())
			if err != nil {
				return exceptions.Cause(err, "download")
			}
			return nil
		})
	case socks.CommandUDPAssociate:
		serverConn, dialErr := system.Dial(ctx, "udp", c.serverAddr.String())
		if dialErr != nil {
			failure()
			return exceptions.Cause(err, "connect to server")
		}
		handler := &udpHandler{
			LocalClient:  c,
			upstreamConn: conn,
			serverConn:   serverConn,
		}
		handler.udpIn = system.NewUDPListener(netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 0), handler)
		handler.udpIn.Start()
		defer handler.Close()
		bindAddr, bindPort := socksaddr.AddressFromNetAddr(handler.udpIn.UDPConn.LocalAddr())
		err = socks.WriteResponse(conn, &socks.Response{
			Version:   request.Version,
			ReplyCode: socks.ReplyCodeSuccess,
			BindAddr:  bindAddr,
			BindPort:  bindPort,
		})
		if err != nil {
			return exceptions.Cause(err, "write response")
		}
		go handler.loopInput()
		return common.Error(io.Copy(io.Discard, conn))
	}

	return nil
}

type udpHandler struct {
	*LocalClient
	upstreamConn net.Conn
	serverConn   net.Conn
	udpIn        *system.UDPListener
	sourceAddr   net.Addr
}

func (c *udpHandler) HandleUDP(listener *system.UDPListener, buffer *buf.Buffer, sourceAddr net.Addr) error {
	c.sourceAddr = sourceAddr
	buffer.Advance(3)
	if c.cipher.SaltSize() > 0 {
		salt := make([]byte, c.cipher.SaltSize())
		common.Must1(rand.Read(salt))
		common.Must1(buffer.WriteAtFirst(salt))
	}
	err := c.cipher.EncodePacket(c.key, buffer)
	if err != nil {
		return exceptions.Cause(err, "encode udp packet")
	}
	defer buffer.Release()
	_, err = c.serverConn.Write(buffer.Bytes())
	if err != nil {
		return exceptions.Cause(err, "write udp packet")
	}
	return nil
}

func (c *udpHandler) loopInput() {
	buffer := buf.New()
	defer buffer.Release()
	for {
		_, err := buffer.ReadFrom(c.serverConn)
		if err != nil {
			c.OnError(exceptions.Cause(err, "read udp packet"))
			return
		}
		err = c.cipher.DecodePacket(c.key, buffer)
		if err != nil {
			c.OnError(exceptions.Cause(err, "decode udp packet"))
			continue
		}
		buffer.ExtendHeader(3) // RSV 2 FRAG 1
		_, err = c.udpIn.WriteTo(buffer.Bytes(), c.sourceAddr)
		if err != nil {
			c.OnError(exceptions.Cause(err, "write back udp packet"))
			return
		}
		buffer.Reset()
	}
}

func (c *udpHandler) OnError(err error) {
	c.LocalClient.OnError(err)
	c.Close()
}

func (c *udpHandler) Close() error {
	c.upstreamConn.Close()
	c.serverConn.Close()
	return nil
}

func (c *LocalClient) OnError(err error) {
	logrus.Warn(err)
}
