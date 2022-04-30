package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/sagernet/sing"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/random"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/protocol/socks"
	"github.com/sagernet/sing/protocol/trojan"
	transTLS "github.com/sagernet/sing/transport/tls"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var configPath string

func main() {
	command := &cobra.Command{
		Use:     "portal-v2board [-c config.json]",
		Args:    cobra.NoArgs,
		Version: sing.VersionStr,
		Run:     run,
	}

	command.Flags().StringVarP(&configPath, "config", "c", "config.json", "set config path")

	if err := command.Execute(); err != nil {
		logrus.Fatal(err)
	}
}

func run(cmd *cobra.Command, args []string) {
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		logrus.Fatal(E.Cause(err, "read config"))
	}
	config := new(Config)
	err = json.Unmarshal(data, config)
	if err != nil {
		logrus.Fatal(E.Cause(err, "parse config"))
	}
	if config.Debug {
		logrus.SetLevel(logrus.TraceLevel)
	}
	if len(config.Nodes) == 0 {
		logrus.Fatal("empty nodes")
	}
	var instances []Instance
	for _, node := range config.Nodes {
		client := NewNodeClient(config.URL, config.Token, strconv.Itoa(node.ID))
		switch node.Type {
		case "trojan":
			instances = append(instances, NewTrojanInstance(client, node))
		default:
			logrus.Fatal("unsupported node type ", node.Type, " (id: ", node.ID, ")")
		}
	}
	for _, instance := range instances {
		err = instance.Start()
		if err != nil {
			logrus.Fatal(err)
		}
	}

	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
	<-osSignals

	for _, instance := range instances {
		instance.Close()
	}
}

type Instance interface {
	Start() error
	Close() error
}

type TrojanInstance struct {
	*NodeClient
	id           int
	domain       string
	listener     net.Listener
	service      trojan.Service[int]
	user         UserManager
	reloadTicker *time.Ticker
}

func NewTrojanInstance(client *NodeClient, node Node) *TrojanInstance {
	t := &TrojanInstance{
		NodeClient: client,
		id:         node.ID,
		domain:     node.Domain,
		user:       NewUserManager(),
	}
	t.service = trojan.NewService[int](t)
	return t
}

func (i *TrojanInstance) Start() error {
	err := i.reloadUsers()
	if err != nil {
		return err
	}

	trojanConfig, err := i.GetTrojanConfig(context.Background())
	if err != nil {
		return E.CauseF(err, i.id, ": read trojan config")
	}

	tcpListener, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: int(trojanConfig.LocalPort),
	})
	if err != nil {
		return E.CauseF(err, i.id, ": listen at tcp:", trojanConfig.LocalPort, ", check server configuration!")
	}

	i.listener = tls.NewListener(tcpListener, &tls.Config{
		Rand: random.Blake3KeyedHash(),
		GetCertificate: func(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return transTLS.GenerateCertificate(info.ServerName)
		},
	})

	logrus.Info(i.id, ": started at ", tcpListener.Addr())
	go i.loopRequests()

	i.reloadTicker = time.NewTicker(time.Minute)
	go i.loopReload()
	return nil
}

func (i *TrojanInstance) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	userCtx := ctx.(*trojan.Context[int])
	conn = i.user.TrackConnection(userCtx.User, conn)
	logrus.Info(i.id, ": user ", userCtx.User, " TCP ", metadata.Source, " ==> ", metadata.Destination)
	destConn, err := network.SystemDialer.DialContext(context.Background(), "tcp", metadata.Destination)
	if err != nil {
		return err
	}
	return rw.CopyConn(ctx, conn, destConn)
}

func (i *TrojanInstance) NewPacketConnection(ctx context.Context, conn socks.PacketConn, metadata M.Metadata) error {
	userCtx := ctx.(*trojan.Context[int])
	conn = i.user.TrackPacketConnection(userCtx.User, conn)
	logrus.Info(i.id, ": user ", userCtx.User, " UDP ", metadata.Source, " ==> ", metadata.Destination)
	udpConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return err
	}
	return socks.CopyNetPacketConn(ctx, udpConn, conn)
}

func (i *TrojanInstance) loopRequests() {
	for {
		conn, err := i.listener.Accept()
		if err != nil {
			logrus.Debug(E.CauseF(err, i.id, ": listener exited"))
			return
		}
		go func() {
			hErr := i.service.NewConnection(context.Background(), conn, M.Metadata{
				Protocol: "tls",
				Source:   M.AddrPortFromNetAddr(conn.RemoteAddr()),
			})
			if hErr != nil {
				i.HandleError(hErr)
			}
		}()
	}
}

func (i *TrojanInstance) loopReload() {
	for range i.reloadTicker.C {
		err := i.reloadUsers()
		if err != nil {
			i.HandleError(E.CauseF(err, "reload user"))
		}
		traffics := i.user.ReadTraffics()
		if len(traffics) > 0 {
			err = i.ReportTrojanTraffic(context.Background(), traffics)
			if err != nil {
				i.HandleError(E.CauseF(err, "report traffic"))
			}
		}
	}
}

func (i *TrojanInstance) reloadUsers() error {
	logrus.Debug(i.id, ": fetching users...")
	userList, err := i.GetTrojanUserList(context.Background())
	if err != nil {
		return E.CauseF(err, i.id, ": get user list")
	}
	if len(userList.Users) == 0 {
		logrus.Warn(i.id, ": empty users")
	}

	i.service.ResetUsers()
	for id, password := range userList.Users {
		err = i.service.AddUser(id, password)
		if err != nil {
			logrus.Warn(E.CauseF(err, i.id, ": add user"))
		}
	}

	logrus.Debug(i.id, ": loaded ", len(userList.Users), " users")
	return nil
}

func (i *TrojanInstance) HandleError(err error) {
	common.Close(err)
	if E.IsClosed(err) {
		return
	}
	logrus.Warn(i.id, ": ", err)
}

func (i *TrojanInstance) Close() error {
	i.reloadTicker.Stop()
	return i.listener.Close()
}
