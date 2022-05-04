//go:build !linux

package redir

import (
	"net"
	"net/netip"

	E "github.com/sagernet/sing/common/exceptions"
)

func TProxy(fd uintptr, isIPv6 bool) error {
	return E.New("only available on linux")
}

func TProxyUDP(fd uintptr, isIPv6 bool) error {
	return E.New("only available on linux")
}

func FWMark(fd uintptr, mark int) error {
	return E.New("only available on linux")
}

func GetOriginalDestinationFromOOB(oob []byte) (netip.AddrPort, error) {
	return netip.AddrPort{}, E.New("only available on linux")
}

func DialUDP(network string, lAddr *net.UDPAddr, rAddr *net.UDPAddr) (*net.UDPConn, error) {
	return nil, E.New("only available on linux")
}
