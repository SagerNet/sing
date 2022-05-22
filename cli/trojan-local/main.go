package main

import (
	"context"
	cTLS "crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/refraction-networking/utls"
	"github.com/sagernet/sing"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	E "github.com/sagernet/sing/common/exceptions"
	_ "github.com/sagernet/sing/common/log"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/redir"
	"github.com/sagernet/sing/protocol/trojan"
	"github.com/sagernet/sing/transport/mixed"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const udpTimeout = 5 * 60

type flags struct {
	Server     string `json:"server"`
	ServerPort uint16 `json:"server_port"`
	ServerName string `json:"server_name"`
	Bind       string `json:"local_address"`
	LocalPort  uint16 `json:"local_port"`
	Password   string `json:"password"`
	Verbose    bool   `json:"verbose"`
	Insecure   bool   `json:"insecure"`
	ConfigFile string
}

func main() {
	f := new(flags)

	command := &cobra.Command{
		Use:     "trojan-local",
		Short:   "trojan client",
		Version: sing.VersionStr,
		Run: func(cmd *cobra.Command, args []string) {
			run(cmd, f)
		},
	}

	command.Flags().StringVarP(&f.Server, "server", "s", "", "Store the server’s hostname or IP.")
	command.Flags().Uint16VarP(&f.ServerPort, "server-port", "p", 0, "Store the server’s port number.")
	command.Flags().StringVarP(&f.ServerName, "server-name", "n", "", "Store the server name.")
	command.Flags().StringVarP(&f.Bind, "local-address", "b", "", "Store the local address.")
	command.Flags().Uint16VarP(&f.LocalPort, "local-port", "l", 0, "Store the local port number.")
	command.Flags().StringVarP(&f.Password, "password", "k", "", "Store the password. The server and the client should use the same password.")
	command.Flags().BoolVarP(&f.Insecure, "insecure", "i", false, "Store insecure.")
	command.Flags().StringVarP(&f.ConfigFile, "config", "c", "", "Use a configuration file.")
	command.Flags().BoolVarP(&f.Verbose, "verbose", "v", false, "Store verbose mode.")

	err := command.Execute()
	if err != nil {
		logrus.Fatal(err)
	}
}

func run(cmd *cobra.Command, f *flags) {
	c, err := newClient(f)
	if err != nil {
		logrus.StandardLogger().Log(logrus.FatalLevel, err, "\n\n")
		cmd.Help()
		os.Exit(1)
	}
	err = c.Start()
	if err != nil {
		logrus.Fatal(err)
	}

	logrus.Info("mixed server started at ", c.TCPListener.Addr())

	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
	<-osSignals

	c.Close()
}

type client struct {
	*mixed.Listener
	server   M.Socksaddr
	key      [trojan.KeyLength]byte
	sni      string
	insecure bool
	dialer   net.Dialer
}

func newClient(f *flags) (*client, error) {
	if f.ConfigFile != "" {
		configFile, err := ioutil.ReadFile(f.ConfigFile)
		if err != nil {
			return nil, E.Cause(err, "read config file")
		}
		flagsNew := new(flags)
		err = json.Unmarshal(configFile, flagsNew)
		if err != nil {
			return nil, E.Cause(err, "decode config file")
		}
		if flagsNew.Server != "" && f.Server == "" {
			f.Server = flagsNew.Server
		}
		if flagsNew.ServerPort != 0 && f.ServerPort == 0 {
			f.ServerPort = flagsNew.ServerPort
		}
		if flagsNew.ServerName != "" && f.ServerName == "" {
			f.ServerName = flagsNew.ServerName
		}
		if flagsNew.Bind != "" && f.Bind == "" {
			f.Bind = flagsNew.Bind
		}
		if flagsNew.LocalPort != 0 && f.LocalPort == 0 {
			f.LocalPort = flagsNew.LocalPort
		}
		if flagsNew.Password != "" && f.Password == "" {
			f.Password = flagsNew.Password
		}
		if flagsNew.Insecure {
			f.Insecure = true
		}
		if flagsNew.Verbose {
			f.Verbose = true
		}
	}

	if f.Verbose {
		logrus.SetLevel(logrus.TraceLevel)
	}

	if f.Server == "" {
		return nil, E.New("missing server address")
	} else if f.ServerPort == 0 {
		return nil, E.New("missing server port")
	}

	c := &client{
		server:   M.ParseSocksaddrHostPort(f.Server, f.ServerPort),
		key:      trojan.Key(f.Password),
		sni:      f.ServerName,
		insecure: f.Insecure,
	}
	if c.sni == "" {
		c.sni = f.Server
	}

	var bind netip.Addr
	if f.Bind != "" {
		addr, err := netip.ParseAddr(f.Bind)
		if err != nil {
			return nil, E.Cause(err, "bad local address")
		}
		bind = addr
	} else {
		bind = netip.IPv6Unspecified()
	}

	c.Listener = mixed.NewListener(netip.AddrPortFrom(bind, f.LocalPort), nil, redir.ModeDisabled, udpTimeout, c)
	return c, nil
}

func (c *client) connect(ctx context.Context) (*cTLS.Conn, error) {
	tcpConn, err := c.dialer.DialContext(ctx, "tcp", c.server.String())
	if err != nil {
		return nil, err
	}

	tlsConn := cTLS.Client(tcpConn, &cTLS.Config{
		ServerName:         c.sni,
		InsecureSkipVerify: c.insecure,
	})
	return tlsConn, nil
}

func (c *client) connectUTLS(ctx context.Context) (*tls.UConn, error) {
	tcpConn, err := c.dialer.DialContext(ctx, "tcp", c.server.String())
	if err != nil {
		return nil, err
	}

	tlsConn := tls.UClient(tcpConn, &tls.Config{
		ServerName:         c.sni,
		InsecureSkipVerify: c.insecure,
	}, tls.HelloCustom)
	clientHelloSpec := tls.ClientHelloSpec{
		CipherSuites: []uint16{
			tls.GREASE_PLACEHOLDER,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.DISABLED_TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
			tls.DISABLED_TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			tls.DISABLED_TLS_RSA_WITH_AES_256_CBC_SHA256,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA,
			0xc008,
			tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
			tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		},
		CompressionMethods: []byte{
			0x00, // compressionNone
		},
		Extensions: []tls.TLSExtension{
			&tls.UtlsGREASEExtension{},
			&tls.SNIExtension{},
			&tls.UtlsExtendedMasterSecretExtension{},
			&tls.RenegotiationInfoExtension{Renegotiation: tls.RenegotiateOnceAsClient},
			&tls.SupportedCurvesExtension{Curves: []tls.CurveID{
				tls.CurveID(tls.GREASE_PLACEHOLDER),
				tls.X25519,
				tls.CurveP256,
				tls.CurveP384,
				tls.CurveP521,
			}},
			&tls.SupportedPointsExtension{SupportedPoints: []byte{
				0x00, // pointFormatUncompressed
			}},
			&tls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}},
			&tls.StatusRequestExtension{},
			&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
				tls.ECDSAWithP256AndSHA256,
				tls.PSSWithSHA256,
				tls.PKCS1WithSHA256,
				tls.ECDSAWithP384AndSHA384,
				tls.ECDSAWithSHA1,
				tls.PSSWithSHA384,
				tls.PSSWithSHA384,
				tls.PKCS1WithSHA384,
				tls.PSSWithSHA512,
				tls.PKCS1WithSHA512,
				tls.PKCS1WithSHA1,
			}},
			&tls.SCTExtension{},
			&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
				{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
				{Group: tls.X25519},
			}},
			&tls.PSKKeyExchangeModesExtension{Modes: []uint8{
				tls.PskModeDHE,
			}},
			&tls.SupportedVersionsExtension{Versions: []uint16{
				tls.GREASE_PLACEHOLDER,
				tls.VersionTLS13,
				tls.VersionTLS12,
				tls.VersionTLS11,
				tls.VersionTLS10,
			}},
			&tls.UtlsGREASEExtension{},
			&tls.UtlsPaddingExtension{GetPaddingLen: tls.BoringPaddingStyle},
		},
	}
	err = tlsConn.ApplyPreset(&clientHelloSpec)
	if err != nil {
		tcpConn.Close()
		return nil, err
	}
	return tlsConn, nil
}

func (c *client) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	logrus.Info("outbound ", metadata.Protocol, " TCP ", conn.RemoteAddr(), " ==> ", metadata.Destination)

	tlsConn, err := c.connect(ctx)
	if err != nil {
		return err
	}
	clientConn := trojan.NewClientConn(tlsConn, c.key, metadata.Destination)

	err = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if err != nil {
		return err
	}

	_request := buf.StackNew()
	request := common.Dup(_request)
	_, err = request.ReadFrom(conn)
	if err != nil && !E.IsTimeout(err) {
		return E.Cause(err, "read payload")
	}

	err = conn.SetReadDeadline(time.Time{})
	if err != nil {
		return err
	}

	_, err = clientConn.Write(request.Bytes())
	if err != nil {
		return E.Cause(err, "client handshake")
	}
	runtime.KeepAlive(_request)
	return bufio.CopyConn(ctx, clientConn, conn)
}

func (c *client) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata M.Metadata) error {
	logrus.Info("outbound ", metadata.Protocol, " UDP ", metadata.Source, " ==> ", metadata.Destination)

	tlsConn, err := c.connect(ctx)
	if err != nil {
		return err
	}
	/*err = trojan.ClientHandshakeRaw(tlsConn, c.key, trojan.CommandUDP, metadata.Destination, nil)
	if err != nil {
		return err
	}
	return socks.CopyPacketConn(ctx, &trojan.PacketConn{Conn: tlsConn}, conn)*/
	clientConn := trojan.NewClientPacketConn(tlsConn, c.key)
	return bufio.CopyPacketConn(ctx, clientConn, conn)
}

func (c *client) HandleError(err error) {
	if E.IsClosed(err) {
		return
	}
	logrus.Warn(err)
}
