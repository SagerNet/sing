package wapd

import (
	"fmt"
	"net/netip"

	"github.com/insomniacslk/dhcp/dhcpv4"
	_ "github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/transport/udp"
	"github.com/sirupsen/logrus"
)

type Listener struct {
	*udp.Listener
	E.Handler
	option dhcpv4.Option
}

func NewListener(bind netip.Addr, proxyURL string, errorHandler E.Handler) *Listener {
	l := &Listener{
		Handler: errorHandler,
		option: dhcpv4.Option{
			Code:  OptionCode,
			Value: dhcpv4.String(fmt.Sprint(proxyURL)),
		},
	}
	l.Listener = udp.NewUDPListener(netip.AddrPortFrom(bind, dhcpv4.ServerPort), l)
	return l
}

func (l *Listener) NewPacket(packet *buf.Buffer, metadata M.Metadata) error {
	request, err := dhcpv4.FromBytes(packet.Bytes())
	if err != nil {
		return E.Cause(err, "bad dhcpv4 packet")
	}
	logrus.Trace("DHCPv4 request ", request)
	if !request.IsOptionRequested(OptionCode) {
		return nil
	}
	reply, err := dhcpv4.NewReplyFromRequest(request, dhcpv4.WithOption(l.option))
	if err != nil {
		return E.Cause(err, "create response")
	}
	_, err = l.WriteTo(reply.ToBytes(), metadata.Source.UDPAddr())
	if err != nil {
		return E.Cause(err, "write response")
	}
	return nil
}
