package socksaddr

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

func ParseAddr(address string) Addr {
	addr, err := netip.ParseAddr(address)
	if err == nil {
		return AddrFromAddr(addr)
	}
	return AddrFromFqdn(address)
}

func ParseAddrPort(address string) (Addr, uint16, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, 0, err
	}
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return nil, 0, err
	}
	return ParseAddr(host), uint16(portInt), nil
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

func AddressFromNetAddr(netAddr net.Addr) (addr Addr, port uint16) {
	var ip net.IP
	switch addr := netAddr.(type) {
	case *net.TCPAddr:
		ip = addr.IP
		port = uint16(addr.Port)
	case *net.UDPAddr:
		ip = addr.IP
		port = uint16(addr.Port)
	}
	return AddrFromIP(ip), port
}

func AddrFromFqdn(fqdn string) Addr {
	return AddrFqdn(fqdn)
}

func JoinHostPort(addr Addr, port uint16) string {
	return net.JoinHostPort(addr.String(), strconv.Itoa(int(port)))
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
