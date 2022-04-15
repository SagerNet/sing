package metadata

import (
	"net"
	"net/netip"
	"strconv"
)

type Addr interface {
	Family() Family
	Addr() netip.Addr
	Fqdn() string
	String() string
}

type AddrPort struct {
	Addr Addr
	Port uint16
}

func (ap AddrPort) IPAddr() *net.IPAddr {
	return &net.IPAddr{
		IP: ap.Addr.Addr().AsSlice(),
	}
}

func (ap AddrPort) TCPAddr() *net.TCPAddr {
	return &net.TCPAddr{
		IP:   ap.Addr.Addr().AsSlice(),
		Port: int(ap.Port),
	}
}

func (ap AddrPort) UDPAddr() *net.UDPAddr {
	return &net.UDPAddr{
		IP:   ap.Addr.Addr().AsSlice(),
		Port: int(ap.Port),
	}
}

func (ap AddrPort) String() string {
	return net.JoinHostPort(ap.Addr.String(), strconv.Itoa(int(ap.Port)))
}

func ParseAddr(address string) Addr {
	addr, err := netip.ParseAddr(address)
	if err == nil {
		return AddrFromAddr(addr)
	}
	return AddrFromFqdn(address)
}

func AddrPortFrom(addr Addr, port uint16) *AddrPort {
	return &AddrPort{addr, port}
}

func ParseAddress(address string) (*AddrPort, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return nil, err
	}
	return AddrPortFrom(ParseAddr(host), uint16(portInt)), nil
}

func ParseAddrPort(address string, port string) (*AddrPort, error) {
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return nil, err
	}
	return AddrPortFrom(ParseAddr(address), uint16(portInt)), nil
}

func AddrFromNetAddr(netAddr net.Addr) Addr {
	switch addr := netAddr.(type) {
	case *net.IPAddr:
		return AddrFromIP(addr.IP)
	case *net.IPNet:
		return AddrFromIP(addr.IP)
	default:
		return nil
	}
}

func AddrPortFromNetAddr(netAddr net.Addr) *AddrPort {
	var ip net.IP
	var port uint16
	switch addr := netAddr.(type) {
	case *net.TCPAddr:
		ip = addr.IP
		port = uint16(addr.Port)
	case *net.UDPAddr:
		ip = addr.IP
		port = uint16(addr.Port)
	case *net.IPAddr:
		ip = addr.IP
	}
	return AddrPortFrom(AddrFromIP(ip), port)
}

func AddrFromIP(ip net.IP) Addr {
	addr, _ := netip.AddrFromSlice(ip)
	if addr.Is4() {
		return Addr4(addr.As4())
	} else {
		return Addr16(addr.As16())
	}
}

func AddrFromAddr(addr netip.Addr) Addr {
	if addr.Is4() {
		return Addr4(addr.As4())
	} else {
		return Addr16(addr.As16())
	}
}

func AddrPortFromAddrPort(addrPort netip.AddrPort) *AddrPort {
	return AddrPortFrom(AddrFromAddr(addrPort.Addr()), addrPort.Port())
}

func AddrFromFqdn(fqdn string) Addr {
	return AddrFqdn(fqdn)
}

type Addr4 [4]byte

func (a Addr4) Family() Family {
	return AddressFamilyIPv4
}

func (a Addr4) Addr() netip.Addr {
	return netip.AddrFrom4(a)
}

func (a Addr4) Fqdn() string {
	return ""
}

func (a Addr4) String() string {
	return net.IP(a[:]).String()
}

type Addr16 [16]byte

func (a Addr16) Family() Family {
	return AddressFamilyIPv6
}

func (a Addr16) Addr() netip.Addr {
	return netip.AddrFrom16(a)
}

func (a Addr16) Fqdn() string {
	return ""
}

func (a Addr16) String() string {
	return net.IP(a[:]).String()
}

type AddrFqdn string

func (f AddrFqdn) Family() Family {
	return AddressFamilyFqdn
}

func (f AddrFqdn) Addr() netip.Addr {
	return netip.Addr{}
}

func (f AddrFqdn) Fqdn() string {
	return string(f)
}

func (f AddrFqdn) String() string {
	return string(f)
}
