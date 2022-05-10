package main

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"runtime"

	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/http3"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	_ "github.com/sagernet/sing/common/log"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/protocol/socks"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/net/dns/dnsmessage"
)

func main() {
	command := &cobra.Command{
		Use:  "socks-chk [socks4/4a/5://]address:port",
		Args: cobra.ExactArgs(1),
		Run:  run,
	}
	if err := command.Execute(); err != nil {
		logrus.Fatal(err)
	}
}

func run(cmd *cobra.Command, args []string) {
	client, err := socks.NewClientFromURL(N.SystemDialer, args[0])
	if err != nil {
		logrus.Fatal(err)
	}
	err = testSocksTCP(client)
	if err != nil {
		logrus.Fatal(err)
	}
	err = testSocksUDP(client)
	if err != nil {
		logrus.Fatal(err)
	}
	err = testSocksQuic(client)
	if err != nil {
		logrus.Fatal(err)
	}
}

func testSocksTCP(client *socks.Client) error {
	tcpConn, err := client.DialContext(context.Background(), "tcp", M.ParseSocksaddrHostPort("1.0.0.1", "53"))
	if err != nil {
		return err
	}

	message := &dnsmessage.Message{}
	message.Header.ID = 1
	message.Header.RecursionDesired = true
	message.Questions = append(message.Questions, dnsmessage.Question{
		Name:  dnsmessage.MustNewName("google.com."),
		Type:  dnsmessage.TypeA,
		Class: dnsmessage.ClassINET,
	})
	packet, err := message.Pack()

	err = binary.Write(tcpConn, binary.BigEndian, uint16(len(packet)))
	if err != nil {
		return err
	}

	_, err = tcpConn.Write(packet)
	if err != nil {
		return err
	}

	var respLen uint16
	err = binary.Read(tcpConn, binary.BigEndian, &respLen)
	if err != nil {
		return err
	}

	respBuf := buf.Make(int(respLen))
	_, err = io.ReadFull(tcpConn, respBuf)
	if err != nil {
		return err
	}

	common.Must(message.Unpack(respBuf))
	for _, answer := range message.Answers {
		logrus.Info("tcp got answer: ", netip.AddrFrom4(answer.Body.(*dnsmessage.AResource).A))
	}

	tcpConn.Close()

	return nil
}

func testSocksUDP(client *socks.Client) error {
	udpConn, err := client.DialContext(context.Background(), "udp", M.ParseSocksaddrHostPort("1.0.0.1", "53"))
	if err != nil {
		return err
	}

	message := &dnsmessage.Message{}
	message.Header.ID = 1
	message.Header.RecursionDesired = true
	message.Questions = append(message.Questions, dnsmessage.Question{
		Name:  dnsmessage.MustNewName("google.com."),
		Type:  dnsmessage.TypeA,
		Class: dnsmessage.ClassINET,
	})
	packet, err := message.Pack()
	common.Must(err)
	common.Must1(udpConn.Write(packet))
	_buffer := buf.StackNew()
	defer runtime.KeepAlive(_buffer)
	buffer := common.Dup(_buffer)
	common.Must1(buffer.ReadFrom(udpConn))
	common.Must(message.Unpack(buffer.Bytes()))

	for _, answer := range message.Answers {
		logrus.Info("udp got answer: ", netip.AddrFrom4(answer.Body.(*dnsmessage.AResource).A))
	}

	udpConn.Close()
	return nil
}

func testSocksQuic(client *socks.Client) error {
	httpClient := &http.Client{
		Transport: &http3.RoundTripper{
			Dial: func(ctx context.Context, network, addr string, tlsCfg *tls.Config, cfg *quic.Config) (quic.EarlyConnection, error) {
				udpAddr, err := net.ResolveUDPAddr(network, addr)
				if err != nil {
					return nil, err
				}
				conn, err := client.DialContext(context.Background(), network, M.SocksaddrFromNet(udpAddr))
				if err != nil {
					return nil, err
				}
				return quic.DialEarlyContext(ctx, conn.(net.PacketConn), udpAddr, M.ParseSocksaddr(addr).AddrString(), tlsCfg, cfg)
			},
		},
	}
	qResponse, err := httpClient.Get("https://cloudflare.com/cdn-cgi/trace")
	if err != nil {
		return err
	}
	qResponse.Write(os.Stderr)
	return nil
}
