package main

import (
	"context"
	"encoding/base64"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/sagernet/sing"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/random"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/protocol/shadowsocks"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead_2022"
	"github.com/sagernet/sing/transport/tcp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type flags struct {
	Bind      string `json:"local_address"`
	LocalPort uint16 `json:"local_port"`
	// Password           string `json:"password"`
	Key        string `json:"key"`
	Method     string `json:"method"`
	Verbose    bool   `json:"verbose"`
	ConfigFile string
}

func main() {
	logrus.SetLevel(logrus.TraceLevel)

	f := new(flags)

	command := &cobra.Command{
		Use:     "ss-local",
		Short:   "shadowsocks client",
		Version: sing.VersionStr,
		Run: func(cmd *cobra.Command, args []string) {
			run(cmd, f)
		},
	}

	command.Flags().StringVarP(&f.Bind, "local-address", "b", "", "Set the local address.")
	command.Flags().Uint16VarP(&f.LocalPort, "local-port", "l", 0, "Set the local port number.")
	command.Flags().StringVarP(&f.Key, "key", "k", "", "Set the key directly. The key should be encoded with URL-safe Base64.")

	var supportedCiphers []string
	supportedCiphers = append(supportedCiphers, shadowsocks.MethodNone)
	supportedCiphers = append(supportedCiphers, shadowaead_2022.List...)

	command.Flags().StringVarP(&f.Method, "encrypt-method", "m", "", "Set the cipher.\n\nSupported ciphers:\n\n"+strings.Join(supportedCiphers, "\n"))
	command.Flags().StringVarP(&f.ConfigFile, "config", "c", "", "Use a configuration file.")
	command.Flags().BoolVarP(&f.Verbose, "verbose", "v", true, "Enable verbose mode.")

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
	logrus.Info("server started at ", s.tcpIn.TCPListener.Addr())

	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
	<-osSignals

	s.tcpIn.Close()
}

type server struct {
	tcpIn   *tcp.Listener
	service shadowsocks.Service
}

func newServer(f *flags) (*server, error) {
	s := new(server)

	if f.Method == shadowsocks.MethodNone {
		s.service = shadowsocks.NewNoneService(s)
	} else if common.Contains(shadowaead_2022.List, f.Method) {
		var pskList [][]byte
		if f.Key != "" {
			keyStrList := strings.Split(f.Key, ":")
			pskList = make([][]byte, len(keyStrList))
			for i, keyStr := range keyStrList {
				key, err := base64.StdEncoding.DecodeString(keyStr)
				if err != nil {
					return nil, E.Cause(err, "decode key")
				}
				pskList[i] = key
			}
		}
		rng := random.System
		service, err := shadowaead_2022.NewService(f.Method, pskList[0], rng, s)
		if err != nil {
			return nil, err
		}
		s.service = service
	} else {
		return nil, E.New("unsupported method " + f.Method)
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
	s.tcpIn = tcp.NewTCPListener(netip.AddrPortFrom(bind, f.LocalPort), s)
	return s, nil
}

func (s *server) NewConnection(conn net.Conn, metadata M.Metadata) error {
	if metadata.Protocol != "shadowsocks" {
		return s.service.NewConnection(conn, metadata)
	}
	logrus.Info("inbound TCP ", conn.RemoteAddr(), " ==> ", metadata.Destination)
	destConn, err := network.SystemDialer.DialContext(context.Background(), "tcp", metadata.Destination)
	if err != nil {
		return err
	}
	return rw.CopyConn(context.Background(), conn, destConn)
}

func (s *server) HandleError(err error) {
	if E.IsClosed(err) {
		return
	}
	logrus.Warn(err)
}
