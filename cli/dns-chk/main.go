package main

import (
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"runtime"

	"github.com/lucas-clemente/quic-go/http3"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	_ "github.com/sagernet/sing/common/log"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/net/dns/dnsmessage"
)

func main() {
	command := &cobra.Command{
		Use: "dns-chk",
		Run: run,
	}
	if err := command.Execute(); err != nil {
		logrus.Fatal(err)
	}
}

func run(cmd *cobra.Command, args []string) {
	err := testSocksTCP()
	if err != nil {
		logrus.Fatal(err)
	}
	err = testSocksUDP()
	if err != nil {
		logrus.Fatal(err)
	}
	err = testQuic()
	if err != nil {
		logrus.Fatal(err)
	}
}

func testSocksTCP() error {
	tcpConn, err := net.Dial("tcp", "1.0.0.1:53")
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

func testSocksUDP() error {
	udpConn, err := net.Dial("udp", "1.0.0.1:53")
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

func testQuic() error {
	client := &http.Client{
		Transport: &http3.RoundTripper{},
	}
	qResponse, err := client.Get("https://cloudflare.com/cdn-cgi/trace")
	if err != nil {
		return err
	}
	qResponse.Write(os.Stderr)
	return nil
}
