package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/sagernet/sing/common/geoip"
	"github.com/sagernet/sing/common/geosite"
	"io"
	"io/ioutil"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"syscall"

	"github.com/sagernet/sing"
	"github.com/sagernet/sing/common"
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

func main() {
	err := MainCmd().Execute()
	if err != nil {
		logrus.Fatal(err)
	}
}

type Flags struct {
	Server      string `json:"server"`
	ServerPort  uint16 `json:"server_port"`
	LocalPort   uint16 `json:"local_port"`
	Password    string `json:"password"`
	Key         string `json:"key"`
	Method      string `json:"method"`
	TCPFastOpen bool   `json:"fast_open"`
	Verbose     bool   `json:"verbose"`
	Redirect    string `json:"redir"`
	FWMark      int    `json:"fwmark"`
	Bypass      string `json:"bypass"`
	ConfigFile  string
}

func MainCmd() *cobra.Command {
	flags := new(Flags)

	cmd := &cobra.Command{
		Use:     "sslocal",
		Short:   "shadowsocks client as socks5 proxy, sing port",
		Version: sing.Version,
		Run: func(cmd *cobra.Command, args []string) {
			Run(cmd, flags)
		},
	}

	cmd.Flags().StringVarP(&flags.Server, "server", "s", "", "Set the server’s hostname or IP.")
	cmd.Flags().Uint16VarP(&flags.ServerPort, "server-port", "p", 0, "Set the server’s port number.")
	cmd.Flags().Uint16VarP(&flags.LocalPort, "local-port", "l", 0, "Set the local port number.")
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
	cmd.Flags().BoolVar(&flags.TCPFastOpen, "fast-open", false, `Enable TCP fast open.
Only available with Linux kernel > 3.7.0.`)
	cmd.Flags().StringVar(&flags.Redirect, "redir", "", "Enable transparent proxy support. [possible values: redirect, tproxy]")
	cmd.Flags().IntVar(&flags.FWMark, "fwmark", 0, "Set outbound socket mark.")
	cmd.Flags().StringVar(&flags.Bypass, "bypass", "", "Set bypass country.")
	cmd.Flags().StringVarP(&flags.ConfigFile, "config", "c", "", "Use a configuration file.")
	cmd.Flags().BoolVarP(&flags.Verbose, "verbose", "v", false, "Enable verbose mode.")

	return cmd
}

type LocalClient struct {
	*system.MixedListener
	*shadowsocks.Client
	*geosite.Matcher
	redirect bool
	bypass   string
}

func NewLocalClient(flags *Flags) (*LocalClient, error) {
	if flags.ConfigFile != "" {
		configFile, err := ioutil.ReadFile(flags.ConfigFile)
		if err != nil {
			return nil, exceptions.Cause(err, "read config file")
		}
		flagsNew := new(Flags)
		err = json.Unmarshal(configFile, flagsNew)
		if err != nil {
			return nil, exceptions.Cause(err, "decode config file")
		}
		if flagsNew.Server != "" && flags.Server == "" {
			flags.Server = flagsNew.Server
		}
		if flagsNew.ServerPort != 0 && flags.ServerPort == 0 {
			flags.ServerPort = flagsNew.ServerPort
		}
		if flagsNew.LocalPort != 0 && flags.LocalPort == 0 {
			flags.LocalPort = flagsNew.LocalPort
		}
		if flagsNew.Password != "" && flags.Password == "" {
			flags.Password = flagsNew.Password
		}
		if flagsNew.Key != "" && flags.Key == "" {
			flags.Key = flagsNew.Key
		}
		if flagsNew.Method != "" && flags.Method == "" {
			flags.Method = flagsNew.Method
		}
		if flagsNew.Redirect != "" && flags.Redirect == "" {
			flags.Redirect = flagsNew.Redirect
		}
		if flagsNew.TCPFastOpen {
			flags.TCPFastOpen = true
		}
		if flagsNew.Verbose {
			flags.Verbose = true
		}

	}

	clientConfig := &shadowsocks.ClientConfig{
		Server:     flags.Server,
		ServerPort: flags.ServerPort,
		Method:     flags.Method,
	}

	if flags.Key != "" {
		key, err := base64.URLEncoding.DecodeString(flags.Key)
		if err != nil {
			return nil, exceptions.Cause(err, "decode key")
		}
		clientConfig.Key = key
	} else if flags.Password != "" {
		clientConfig.Password = []byte(flags.Password)
	}

	if flags.Verbose {
		logrus.SetLevel(logrus.TraceLevel)
	}

	dialer := new(net.Dialer)

	dialer.Control = func(network, address string, c syscall.RawConn) error {
		var rawFd uintptr
		err := c.Control(func(fd uintptr) {
			rawFd = fd
		})
		if err != nil {
			return err
		}
		if flags.FWMark > 0 {
			err = syscall.SetsockoptInt(int(rawFd), syscall.SOL_SOCKET, syscall.SO_MARK, flags.FWMark)
			if err != nil {
				return err
			}
		}
		if flags.TCPFastOpen {
			err = system.TCPFastOpen(rawFd)
			if err != nil {
				return err
			}
		}
		return nil
	}

	shadowClient, err := shadowsocks.NewClient(dialer, clientConfig)
	if err != nil {
		return nil, err
	}

	client := &LocalClient{
		Client: shadowClient,
	}
	client.MixedListener = system.NewMixedListener(netip.AddrPortFrom(netip.IPv6Unspecified(), flags.LocalPort), &system.MixedConfig{
		Redirect: flags.Redirect == "redirect",
		TProxy:   flags.Redirect == "tproxy",
	}, client)

	if flags.Bypass != "" {
		client.bypass = flags.Bypass

		err = geoip.LoadMMDB("Country.mmdb")
		if err != nil {
			return nil, exceptions.Cause(err, "load Country.mmdb")
		}

		geodata, err := os.Open("geosite.dat")
		if err != nil {
			return nil, exceptions.Cause(err, "geosite.dat not found")
		}

		geositeMatcher, err := geosite.LoadGeositeMatcher(geodata, flags.Bypass)
		if err != nil {
			return nil, err
		}
		client.Matcher = geositeMatcher
		debug.FreeOSMemory()
	}

	return client, nil
}

func (c *LocalClient) Start() error {
	err := c.MixedListener.Start()
	if err != nil {
		return err
	}
	logrus.Info("mixed server started at ", c.MixedListener.TCPListener.Addr())
	return nil
}

func bypass(addr socksaddr.Addr, port uint16, conn net.Conn) error {
	logrus.Info("BYPASS ", conn.RemoteAddr(), " ==> ", net.JoinHostPort(addr.String(), strconv.Itoa(int(port))))
	serverConn, err := net.Dial("tcp", socksaddr.JoinHostPort(addr, port))
	if err != nil {
		return err
	}
	return task.Run(context.Background(), func() error {
		defer rw.CloseRead(conn)
		defer rw.CloseWrite(serverConn)
		return common.Error(io.Copy(serverConn, conn))
	}, func() error {
		defer rw.CloseRead(serverConn)
		defer rw.CloseWrite(conn)
		return common.Error(io.Copy(conn, serverConn))
	})
}

func (c *LocalClient) NewConnection(addr socksaddr.Addr, port uint16, conn net.Conn) error {
	if c.bypass != "" {
		if addr.Family().IsFqdn() {
			if c.Match(addr.Fqdn()) {
				return bypass(addr, port, conn)
			}
		} else {
			if geoip.Match(c.bypass, addr.Addr().AsSlice()) {
				return bypass(addr, port, conn)
			}
		}
	}

	logrus.Info("CONNECT ", conn.RemoteAddr(), " ==> ", net.JoinHostPort(addr.String(), strconv.Itoa(int(port))))

	ctx := context.Background()
	serverConn, err := c.DialContextTCP(ctx, addr, port)
	if err != nil {
		return err
	}
	return task.Run(ctx, func() error {
		defer rw.CloseRead(conn)
		defer rw.CloseWrite(serverConn)
		return common.Error(io.Copy(serverConn, conn))
	}, func() error {
		defer rw.CloseRead(serverConn)
		defer rw.CloseWrite(conn)
		return common.Error(io.Copy(conn, serverConn))
	})
}

func (c *LocalClient) NewPacketConnection(conn socks.PacketConn, addr socksaddr.Addr, port uint16) error {
	ctx := context.Background()
	serverConn := c.DialContextUDP(ctx)
	return task.Run(ctx, func() error {
		var init bool
		return socks.CopyPacketConn(serverConn, conn, func(size int) {
			if !init {
				init = true
				logrus.Info("UDP ", conn.LocalAddr(), " ==> ", socksaddr.JoinHostPort(addr, port))
			} else {
				logrus.Trace("UDP ", conn.LocalAddr(), " ==> ", socksaddr.JoinHostPort(addr, port))
			}
		})
	}, func() error {
		return socks.CopyPacketConn(conn, serverConn, func(size int) {
			logrus.Trace("UDP ", conn.LocalAddr(), " <== ", socksaddr.JoinHostPort(addr, port))
		})
	})
}

func Run(cmd *cobra.Command, flags *Flags) {
	client, err := NewLocalClient(flags)
	if err != nil {
		logrus.StandardLogger().Log(logrus.FatalLevel, err, "\n\n")
		cmd.Help()
		os.Exit(1)
	}
	err = client.Start()
	if err != nil {
		logrus.Fatal(err)
	}
	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
	<-osSignals
	client.Close()
}

func (c *LocalClient) OnError(err error) {
	if exceptions.IsClosed(err) {
		return
	}
	logrus.Warn(err)
}
