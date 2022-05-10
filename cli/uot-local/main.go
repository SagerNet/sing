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
	_ "github.com/sagernet/sing/common/log"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
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

	socks, err := socks.NewClientFromURL(N.SystemDialer, args[1])
	if err != nil {
		logrus.Fatal(err)
	}

	client := &localClient{upstream: socks}
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
	upstream *socks.Client
}

func (c *localClient) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	logrus.Info("inbound ", metadata.Protocol, " TCP ", metadata.Source, " ==> ", metadata.Destination)

	upstream, err := c.upstream.DialContext(ctx, "tcp", metadata.Destination)
	if err != nil {
		return err
	}

	return rw.CopyConn(context.Background(), conn, upstream)
}

func (c *localClient) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata M.Metadata) error {
	logrus.Info("inbound ", metadata.Protocol, " UDP ", metadata.Source, " ==> ", metadata.Destination)

	upstream, err := c.upstream.DialContext(ctx, "tcp", metadata.Destination)
	if err != nil {
		return err
	}

	return N.CopyPacketConn(ctx, conn, uot.NewClientConn(upstream))
}

func (c *localClient) OnError(err error) {
	common.Close(err)
	if E.IsClosed(err) {
		return
	}
	logrus.Warn(err)
}
