package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"syscall"

	"github.com/sagernet/sing"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/random"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/protocol/socks"
	"github.com/sagernet/sing/protocol/trojan"
	"github.com/sagernet/sing/transport/tcp"
	transTLS "github.com/sagernet/sing/transport/tls"
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

	command.Flags().StringVarP(&f.Server, "server", "s", "", "Set the server’s hostname or IP.")
	command.Flags().Uint16VarP(&f.ServerPort, "server-port", "p", 0, "Set the server’s port number.")
	command.Flags().StringVarP(&f.Bind, "local-address", "b", "", "Set the local address.")
	command.Flags().Uint16VarP(&f.LocalPort, "local-port", "l", 0, "Set the local port number.")
	command.Flags().StringVarP(&f.Password, "password", "k", "", "Set the password. The server and the client should use the same password.")
	command.Flags().BoolVarP(&f.Insecure, "insecure", "i", false, "Set insecure.")
	command.Flags().StringVarP(&f.ConfigFile, "config", "c", "", "Use a configuration file.")
	command.Flags().BoolVarP(&f.Verbose, "verbose", "v", false, "Set verbose mode.")

	err := command.Execute()
	if err != nil {
		logrus.Fatal(err)
	}
}

func run(cmd *cobra.Command, f *flags) {
	c, err := newServer(f)
	if err != nil {
		logrus.StandardLogger().Log(logrus.FatalLevel, err, "\n\n")
		cmd.Help()
		os.Exit(1)
	}
	err = c.tcpIn.Start()
	if err != nil {
		logrus.Fatal(err)
	}

	logrus.Info("server started at ", c.tcpIn.Addr())

	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
	<-osSignals

	c.tcpIn.Close()
}

type server struct {
	tcpIn   *tcp.Listener
	service trojan.Service[int]
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
	s.service = trojan.NewService[int](s)
	common.Must(s.service.AddUser(0, f.Password))
	s.tcpIn = tcp.NewTCPListener(netip.AddrPortFrom(bind, f.ServerPort), s)
	return s, nil
}

func (s *server) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	if metadata.Protocol != "trojan" {
		logrus.Trace("inbound raw TCP from ", metadata.Source)
		tlsConn := tls.Server(conn, &tls.Config{
			Rand: random.Blake3KeyedHash(),
			GetCertificate: func(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
				return transTLS.GenerateCertificate(info.ServerName)
			},
		})
		return s.service.NewConnection(ctx, tlsConn, metadata)
	}
	destConn, err := network.SystemDialer.DialContext(context.Background(), "tcp", metadata.Destination)
	if err != nil {
		return err
	}
	logrus.Info("inbound TCP ", conn.RemoteAddr(), " ==> ", metadata.Destination)
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

func (s *server) HandleError(err error) {
	common.Close(err)
	if E.IsClosed(err) {
		return
	}
	logrus.Warn(err)
}
