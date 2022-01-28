package socksaddr

import (
	"net"

	"net/netip"
)

type Addr interface {
	Family() Family
	Addr() netip.Addr
	Fqdn() string
}

func AddrFromIP(ip net.IP) Addr {
	addr, _ := netip.AddrFromSlice(ip)
	if addr.Is4() {
		return Addr4(addr.As4())
	} else {
		return Addr16(addr.As16())
	}
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
