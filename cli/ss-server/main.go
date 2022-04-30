package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/sagernet/sing"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/random"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/protocol/shadowsocks"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead_2022"
	"github.com/sagernet/sing/protocol/socks"
	"github.com/sagernet/sing/transport/tcp"
	"github.com/sagernet/sing/transport/udp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const udpTimeout = 5 * 60

type flags struct {
	Server     string `json:"server"`
	ServerPort uint16 `json:"server_port"`
	Bind       string `json:"local_address"`
	LocalPort  uint16 `json:"local_port"`
	Password   string `json:"password"`
	Key        string `json:"key"`
	Method     string `json:"method"`
	Verbose    bool   `json:"verbose"`
	ConfigFile string
}

func main() {
	f := new(flags)

	command := &cobra.Command{
		Use:     "ss-local",
		Short:   "shadowsocks client",
		Version: sing.VersionStr,
		Run: func(cmd *cobra.Command, args []string) {
			run(cmd, f)
		},
	}

	command.Flags().StringVarP(&f.Server, "server", "s", "", "Set the server’s hostname or IP.")
	command.Flags().Uint16VarP(&f.ServerPort, "server-port", "p", 0, "Set the server’s port number.")
	command.Flags().StringVarP(&f.Bind, "local-address", "b", "", "Set the local address.")
	command.Flags().Uint16VarP(&f.LocalPort, "local-port", "l", 0, "Set the local port number.")
	command.Flags().StringVar(&f.Key, "key", "", "Set the key directly. The key should be encoded with URL-safe Base64.")
	command.Flags().StringVarP(&f.Password, "password", "k", "", "Set the password. The server and the client should use the same password.")

	var supportedCiphers []string
	supportedCiphers = append(supportedCiphers, shadowsocks.MethodNone)
	supportedCiphers = append(supportedCiphers, shadowaead.List...)
	supportedCiphers = append(supportedCiphers, shadowaead_2022.List...)

	command.Flags().StringVarP(&f.Method, "encrypt-method", "m", "", "Set the cipher.\n\nSupported ciphers:\n\n"+strings.Join(supportedCiphers, "\n"))
	command.Flags().StringVarP(&f.ConfigFile, "config", "c", "", "Use a configuration file.")
	command.Flags().BoolVarP(&f.Verbose, "verbose", "v", false, "Enable verbose mode.")

	err := command.Execute()
	if err != nil {
		logrus.Fatal(err)
	}
}

func run(cmd *cobra.Command, f *flags) {
	s, err := newServer(f)
	if err != nil {
		logrus.Fatal(err)
	}
	err = s.tcpIn.Start()
	if err != nil {
		logrus.Fatal(err)
	}
	err = s.udpIn.Start()
	if err != nil {
		logrus.Fatal(err)
	}
	logrus.Info("server started at ", s.tcpIn.TCPListener.Addr())

	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
	<-osSignals

	s.tcpIn.Close()
	s.udpIn.Close()
}

type server struct {
	tcpIn   *tcp.Listener
	udpIn   *udp.Listener
	service shadowsocks.Service
}

func (s *server) Start() error {
	err := s.tcpIn.Start()
	if err != nil {
		return err
	}
	err = s.udpIn.Start()
	return err
}

func (s *server) Close() error {
	s.tcpIn.Close()
	s.udpIn.Close()
	return nil
}

func newServer(f *flags) (*server, error) {
	s := new(server)

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
		if flagsNew.Bind != "" && f.Bind == "" {
			f.Bind = flagsNew.Bind
		}
		if flagsNew.LocalPort != 0 && f.LocalPort == 0 {
			f.LocalPort = flagsNew.LocalPort
		}
		if flagsNew.Password != "" && f.Password == "" {
			f.Password = flagsNew.Password
		}
		if flagsNew.Key != "" && f.Key == "" {
			f.Key = flagsNew.Key
		}
		if flagsNew.Method != "" && f.Method == "" {
			f.Method = flagsNew.Method
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
	} else if f.Method == "" {
		return nil, E.New("missing method")
	}

	if f.Method == shadowsocks.MethodNone {
		s.service = shadowsocks.NewNoneService(udpTimeout, s)
	} else if common.Contains(shadowaead.List, f.Method) {
		var key []byte
		if f.Key != "" {
			kb, err := base64.StdEncoding.DecodeString(f.Key)
			if err != nil {
				return nil, E.Cause(err, "decode key")
			}
			key = kb
		}
		service, err := shadowaead.NewService(f.Method, key, []byte(f.Password), random.Blake3KeyedHash(), false, udpTimeout, s)
		if err != nil {
			return nil, err
		}
		s.service = service
	} else if common.Contains(shadowaead_2022.List, f.Method) {
		var key [shadowaead_2022.KeySaltSize]byte
		if f.Key != "" {
			kb, err := base64.StdEncoding.DecodeString(f.Key)
			if err != nil {
				return nil, E.Cause(err, "decode key")
			}
			if len(kb) != shadowaead_2022.KeySaltSize {
				return nil, shadowaead.ErrBadKey
			}
			copy(key[:], kb)
		}
		service, err := shadowaead_2022.NewService(f.Method, key, random.Blake3KeyedHash(), udpTimeout, s)
		if err != nil {
			return nil, err
		}
		s.service = service
	} else {
		return nil, E.New("unsupported method " + f.Method)
	}

	var bind netip.Addr
	if f.Server != "" {
		addr, err := netip.ParseAddr(f.Server)
		if err != nil {
			return nil, E.Cause(err, "bad server address")
		}
		bind = addr
	} else {
		bind = netip.IPv6Unspecified()
	}
	s.tcpIn = tcp.NewTCPListener(netip.AddrPortFrom(bind, f.ServerPort), s)
	s.udpIn = udp.NewUDPListener(netip.AddrPortFrom(bind, f.ServerPort), s)
	return s, nil
}

func (s *server) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	if metadata.Protocol != "shadowsocks" {
		logrus.Trace("inbound raw TCP from ", metadata.Source)
		return s.service.NewConnection(ctx, conn, metadata)
	}
	logrus.Info("inbound TCP ", conn.RemoteAddr(), " ==> ", metadata.Destination)
	destConn, err := network.SystemDialer.DialContext(context.Background(), "tcp", metadata.Destination)
	if err != nil {
		return err
	}
	return rw.CopyConn(ctx, conn, destConn)
}

func (s *server) NewPacketConnection(ctx context.Context, conn socks.PacketConn, metadata M.Metadata) error {
	logrus.Info("inbound UDP ", metadata.Source, " ==> ", metadata.Destination)
	udpConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return err
	}
	return socks.CopyNetPacketConn(ctx, udpConn, conn)
}

func (s *server) NewPacket(conn socks.PacketConn, buffer *buf.Buffer, metadata M.Metadata) error {
	logrus.Trace("inbound raw UDP from ", metadata.Source)
	return s.service.NewPacket(conn, buffer, metadata)
}

func (s *server) HandleError(err error) {
	if E.IsClosed(err) {
		return
	}
	logrus.Warn(err)
}
