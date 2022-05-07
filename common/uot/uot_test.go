package uot

import (
	"net"
	"runtime"
	"testing"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"golang.org/x/net/dns/dnsmessage"
)

func TestServerConn(t *testing.T) {
	udpConn, err := net.ListenUDP("udp", nil)
	common.Must(err)
	serverConn := NewServerConn(udpConn)
	defer serverConn.Close()
	clientConn := NewClientConn(serverConn)
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
	common.Must1(clientConn.WriteTo(packet, &net.UDPAddr{
		IP:   net.IPv4(8, 8, 8, 8),
		Port: 53,
	}))
	_buffer := buf.StackNew()
	defer runtime.KeepAlive(_buffer)
	buffer := common.Dup(_buffer)
	common.Must2(buffer.ReadPacketFrom(clientConn))
	common.Must(message.Unpack(buffer.Bytes()))
	for _, answer := range message.Answers {
		t.Log("got answer :", answer.Body)
	}
}
