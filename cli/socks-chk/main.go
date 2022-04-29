package main

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/netip"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/protocol/socks"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/net/dns/dnsmessage"
)

func main() {
	command := &cobra.Command{
		Use:  "socks-chk address:port",
		Args: cobra.ExactArgs(1),
		Run:  run,
	}
	if err := command.Execute(); err != nil {
		logrus.Fatal(err)
	}
}

func run(cmd *cobra.Command, args []string) {
	server, err := M.ParseAddress(args[0])
	if err != nil {
		logrus.Fatal("invalid server address ", args[0])
	}
	err = testSocksTCP(server)
	if err != nil {
		logrus.Fatal(err)
	}
	err = testSocksUDP(server)
	if err != nil {
		logrus.Fatal(err)
	}
}

func testSocksTCP(server *M.AddrPort) error {
	tcpConn, err := net.Dial("tcp", server.String())
	if err != nil {
		return err
	}
	response, err := socks.ClientHandshake(tcpConn, socks.Version5, socks.CommandConnect, M.AddrPortFrom(M.ParseAddr("1.0.0.1"), 53), "", "")
	if err != nil {
		return err
	}
	if response.ReplyCode != socks.ReplyCodeSuccess {
		logrus.Fatal("socks tcp handshake failure: ", response.ReplyCode)
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

func testSocksUDP(server *M.AddrPort) error {
	tcpConn, err := net.Dial("tcp", server.String())
	if err != nil {
		return err
	}
	dest := M.AddrPortFrom(M.ParseAddr("1.0.0.1"), 53)
	response, err := socks.ClientHandshake(tcpConn, socks.Version5, socks.CommandUDPAssociate, dest, "", "")
	if err != nil {
		return err
	}
	if response.ReplyCode != socks.ReplyCodeSuccess {
		logrus.Fatal("socks tcp handshake failure: ", response.ReplyCode)
	}
	var dialer net.Dialer
	udpConn, err := dialer.DialContext(context.Background(), "udp", response.Bind.String())
	if err != nil {
		return err
	}
	assConn := socks.NewAssociateConn(tcpConn, udpConn, dest)
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
	common.Must1(assConn.WriteTo(packet, &net.UDPAddr{
		IP:   net.IPv4(1, 0, 0, 1),
		Port: 53,
	}))
	_buffer := buf.StackNew()
	buffer := common.Dup(_buffer)
	common.Must2(buffer.ReadPacketFrom(assConn))
	common.Must(message.Unpack(buffer.Bytes()))

	for _, answer := range message.Answers {
		logrus.Info("udp got answer: ", netip.AddrFrom4(answer.Body.(*dnsmessage.AResource).A))
	}

	udpConn.Close()
	tcpConn.Close()
	return nil
}
