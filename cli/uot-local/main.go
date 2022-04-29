package main

import (
	"context"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"syscall"

	"github.com/sagernet/sing"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/redir"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/common/uot"
	"github.com/sagernet/sing/protocol/socks"
	"github.com/sagernet/sing/transport/mixed"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	verbose    bool
	transproxy string
)

func main() {
	command := cobra.Command{
		Use:     "uot-local <bind> <upstream>",
		Short:   "SUoT client.",
		Long:    "SUoT client. \n\nconverts a normal socks server to a SUoT mixed server.",
		Example: "uot-local 0.0.0.0:2080 127.0.0.1:1080",
		Version: sing.VersionStr,
		Args:    cobra.ExactArgs(2),
		Run:     run,
	}
	command.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose mode")
	command.Flags().StringVarP(&transproxy, "transproxy", "t", "", "Enable transparent proxy support [possible values: redirect, tproxy]")
	err := command.Execute()
	if err != nil {
		logrus.Fatal(err)
	}
}

func run(cmd *cobra.Command, args []string) {
	if verbose {
		logrus.SetLevel(logrus.TraceLevel)
	}

	bind, err := netip.ParseAddrPort(args[0])
	if err != nil {
		logrus.Fatal("bad bind address: ", err)
	}

	_, err = netip.ParseAddrPort(args[1])
	if err != nil {
		logrus.Fatal("bad upstream address: ", err)
	}

	var transproxyMode redir.TransproxyMode
	switch transproxy {
	case "redirect":
		transproxyMode = redir.ModeRedirect
	case "tproxy":
		transproxyMode = redir.ModeTProxy
	case "":
		transproxyMode = redir.ModeDisabled
	default:
		logrus.Fatal("unknown transproxy mode ", transproxy)
	}

	client := &localClient{upstream: args[1]}
	client.Listener = mixed.NewListener(bind, nil, transproxyMode, 300, client)

	err = client.Start()
	if err != nil {
		logrus.Fatal("start mixed server: ", err)
	}

	logrus.Info("mixed server started at ", client.TCPListener.Addr())

	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
	<-osSignals

	client.Close()
}

type localClient struct {
	*mixed.Listener
	upstream string
}

func (c *localClient) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	logrus.Info("CONNECT ", conn.RemoteAddr(), " ==> ", metadata.Destination)

	upstream, err := net.Dial("tcp", c.upstream)
	if err != nil {
		return E.Cause(err, "connect to upstream")
	}

	_, err = socks.ClientHandshake(upstream, socks.Version5, socks.CommandConnect, metadata.Destination, "", "")
	if err != nil {
		return E.Cause(err, "upstream handshake failed")
	}

	return rw.CopyConn(context.Background(), upstream, conn)
}

func (c *localClient) NewPacketConnection(conn socks.PacketConn, _ M.Metadata) error {
	upstream, err := net.Dial("tcp", c.upstream)
	if err != nil {
		return E.Cause(err, "connect to upstream")
	}

	_, err = socks.ClientHandshake(upstream, socks.Version5, socks.CommandConnect, M.AddrPortFrom(M.AddrFromFqdn(uot.UOTMagicAddress), 443), "", "")
	if err != nil {
		return E.Cause(err, "upstream handshake failed")
	}

	client := uot.NewClientConn(upstream)
	return socks.CopyPacketConn(context.Background(), client, conn)
}

func (c *localClient) OnError(err error) {
	common.Close(err)
	if E.IsClosed(err) {
		return
	}
	logrus.Warn(err)
}
