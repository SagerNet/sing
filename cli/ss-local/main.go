package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/ioutil"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/sagernet/sing"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/geoip"
	"github.com/sagernet/sing/common/geosite"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/random"
	"github.com/sagernet/sing/common/redir"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/common/task"
	"github.com/sagernet/sing/protocol/shadowsocks"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead_2022"
	"github.com/sagernet/sing/protocol/socks"
	"github.com/sagernet/sing/transport/mixed"
	"github.com/sagernet/sing/transport/system"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type flags struct {
	Server             string `json:"server"`
	ServerPort         uint16 `json:"server_port"`
	Bind               string `json:"local_address"`
	LocalPort          uint16 `json:"local_port"`
	Password           string `json:"password"`
	Key                string `json:"key"`
	Method             string `json:"method"`
	TCPFastOpen        bool   `json:"fast_open"`
	Verbose            bool   `json:"verbose"`
	Transproxy         string `json:"transproxy"`
	FWMark             int    `json:"fwmark"`
	Bypass             string `json:"bypass"`
	UseSystemRNG       bool   `json:"use_system_rng"`
	ReducedSaltEntropy bool   `json:"reduced_salt_entropy"`
	ConfigFile         string
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
	command.Flags().StringVarP(&f.Password, "password", "k", "", "Set the password. The server and the client should use the same password.")
	command.Flags().StringVar(&f.Key, "key", "", "Set the key directly. The key should be encoded with URL-safe Base64.")

	var supportedCiphers []string
	supportedCiphers = append(supportedCiphers, shadowsocks.MethodNone)
	supportedCiphers = append(supportedCiphers, shadowaead_2022.List...)
	supportedCiphers = append(supportedCiphers, shadowaead.List...)

	command.Flags().StringVarP(&f.Method, "encrypt-method", "m", "", "Set the cipher.\n\nSupported ciphers:\n\n"+strings.Join(supportedCiphers, "\n"))
	command.Flags().BoolVar(&f.TCPFastOpen, "fast-open", false, `Enable TCP fast open.
Only available with Linux kernel > 3.7.0.`)
	command.Flags().StringVarP(&f.Transproxy, "transproxy", "t", "", "Enable transparent proxy support. [possible values: redirect, tproxy]")
	command.Flags().IntVar(&f.FWMark, "fwmark", 0, "Set outbound socket mark.")
	command.Flags().StringVar(&f.Bypass, "bypass", "", "Set bypass country.")
	command.Flags().StringVarP(&f.ConfigFile, "config", "c", "", "Use a configuration file.")
	command.Flags().BoolVarP(&f.Verbose, "verbose", "v", true, "Enable verbose mode.")
	command.Flags().BoolVar(&f.UseSystemRNG, "use-system-rng", false, "Use system random number generator.")
	command.Flags().BoolVar(&f.ReducedSaltEntropy, "reduced-salt-entropy", false, "Remapping salt to printable chars.")

	err := command.Execute()
	if err != nil {
		logrus.Fatal(err)
	}
}

type client struct {
	*mixed.Listener
	*geosite.Matcher
	server *M.AddrPort
	method shadowsocks.Method
	dialer net.Dialer
	bypass string
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
		if flagsNew.Transproxy != "" && f.Transproxy == "" {
			f.Transproxy = flagsNew.Transproxy
		}
		if flagsNew.TCPFastOpen {
			f.TCPFastOpen = true
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

	c := &client{
		server: M.AddrPortFrom(M.ParseAddr(f.Server), f.ServerPort),
		bypass: f.Bypass,
	}

	if f.Method == shadowsocks.MethodNone {
		c.method = shadowsocks.NewNone()
	} else {
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
		var rng io.Reader
		if f.UseSystemRNG {
			rng = random.System
		} else {
			rng = random.System
		}
		if f.ReducedSaltEntropy {
			rng = &shadowsocks.ReducedEntropyReader{Reader: rng}
		}
		if common.Contains(shadowaead.List, f.Method) {
			if len(pskList) > 1 {
				return nil, shadowaead.ErrBadKey
			}
			var key []byte
			if len(pskList) > 0 {
				key = pskList[0]
			}
			method, err := shadowaead.New(f.Method, key, []byte(f.Password), rng, false)
			if err != nil {
				return nil, err
			}
			c.method = method
		} else if common.Contains(shadowaead_2022.List, f.Method) {
			method, err := shadowaead_2022.New(f.Method, pskList, rng)
			if err != nil {
				return nil, err
			}
			c.method = method
		}
	}

	c.dialer.Control = func(network, address string, c syscall.RawConn) error {
		var rawFd uintptr
		err := c.Control(func(fd uintptr) {
			rawFd = fd
		})
		if err != nil {
			return err
		}
		if f.FWMark > 0 {
			err = redir.FWMark(rawFd, f.FWMark)
			if err != nil {
				return err
			}
		}
		if f.TCPFastOpen {
			err = system.TCPFastOpen(rawFd)
			if err != nil {
				return err
			}
		}
		return nil
	}

	var transproxyMode redir.TransproxyMode
	switch f.Transproxy {
	case "redirect":
		transproxyMode = redir.ModeRedirect
	case "tproxy":
		transproxyMode = redir.ModeTProxy
	case "":
		transproxyMode = redir.ModeDisabled
	default:
		return nil, E.New("unknown transproxy mode ", f.Transproxy)
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

	c.Listener = mixed.NewListener(netip.AddrPortFrom(bind, f.LocalPort), nil, transproxyMode, c)

	if f.Bypass != "" {
		err := geoip.LoadMMDB("Country.mmdb")
		if err != nil {
			return nil, E.Cause(err, "load Country.mmdb")
		}

		geodata, err := os.Open("geosite.dat")
		if err != nil {
			return nil, E.Cause(err, "geosite.dat not found")
		}

		site, err := geosite.ReadArray(geodata, f.Bypass)
		if err != nil {
			return nil, err
		}

		geositeMatcher, err := geosite.NewMatcher(site)
		if err != nil {
			return nil, err
		}
		c.Matcher = geositeMatcher
	}
	debug.FreeOSMemory()

	return c, nil
}

func bypass(conn net.Conn, destination *M.AddrPort) error {
	logrus.Info("BYPASS ", conn.RemoteAddr(), " ==> ", destination)
	serverConn, err := net.Dial("tcp", destination.String())
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

func (c *client) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	if c.bypass != "" {
		if metadata.Destination.Addr.Family().IsFqdn() {
			if c.Match(metadata.Destination.Addr.Fqdn()) {
				return bypass(conn, metadata.Destination)
			}
		} else {
			if geoip.Match(c.bypass, metadata.Destination.Addr.Addr().AsSlice()) {
				return bypass(conn, metadata.Destination)
			}
		}
	}

	logrus.Info("outbound ", metadata.Protocol, " TCP ", conn.RemoteAddr(), " ==> ", metadata.Destination)

	serverConn, err := c.dialer.DialContext(ctx, "tcp", c.server.String())
	if err != nil {
		return E.Cause(err, "connect to server")
	}
	_payload := buf.StackNew()
	payload := common.Dup(_payload)
	err = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if err != nil {
		return err
	}
	_, err = payload.ReadFrom(conn)
	if err != nil && !E.IsTimeout(err) {
		return E.Cause(err, "read payload")
	}
	err = conn.SetReadDeadline(time.Time{})
	if err != nil {
		payload.Release()
		return err
	}
	serverConn = c.method.DialEarlyConn(serverConn, metadata.Destination)
	_, err = serverConn.Write(payload.Bytes())
	if err != nil {
		return E.Cause(err, "client handshake")
	}

	return rw.CopyConn(ctx, serverConn, conn)
}

func (c *client) NewPacketConnection(conn socks.PacketConn, _ M.Metadata) error {
	ctx := context.Background()
	udpConn, err := c.dialer.DialContext(ctx, "udp", c.server.String())
	if err != nil {
		return err
	}
	serverConn := c.method.DialPacketConn(udpConn)
	return task.Run(ctx, func() error {
		var init bool
		return socks.CopyPacketConn0(serverConn, conn, func(destination *M.AddrPort, n int) {
			if !init {
				init = true
				logrus.Info("UDP ", conn.LocalAddr(), " ==> ", destination)
			} else {
				logrus.Trace("UDP ", conn.LocalAddr(), " ==> ", destination)
			}
		})
	}, func() error {
		return socks.CopyPacketConn0(conn, serverConn, func(destination *M.AddrPort, n int) {
			logrus.Trace("UDP ", conn.LocalAddr(), " <== ", destination)
		})
	})
}

func run(cmd *cobra.Command, flags *flags) {
	c, err := newClient(flags)
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

func (c *client) HandleError(err error) {
	common.Close(err)
	if E.IsClosed(err) {
		return
	}
	logrus.Warn(err)
}
