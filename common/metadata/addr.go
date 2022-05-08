package metadata

import (
	"net"
	"net/netip"
	"strconv"
)

type Socksaddr struct {
	Addr netip.Addr
	Fqdn string
	Port uint16
}

func (ap Socksaddr) Network() string {
	return "socks"
}

func (ap Socksaddr) IsIP() bool {
	return ap.Addr.IsValid()
}

func (ap Socksaddr) IsFqdn() bool {
	return !ap.IsIP()
}

func (ap Socksaddr) IsValid() bool {
	return ap.Addr.IsValid() || ap.Fqdn != ""
}

func (ap Socksaddr) Family() Family {
	if ap.Addr.IsValid() {
		if ap.Addr.Is4() {
			return AddressFamilyIPv4
		} else {
			return AddressFamilyIPv6
		}
	}
	if ap.Fqdn != "" {
		return AddressFamilyFqdn
	} else if ap.Addr.Is4() || ap.Addr.Is4In6() {
		return AddressFamilyIPv4
	} else {
		return AddressFamilyIPv6
	}
}

func (ap Socksaddr) AddrString() string {
	if ap.Addr.IsValid() {
		return ap.Addr.String()
	} else {
		return ap.Fqdn
	}
}

func (ap Socksaddr) IPAddr() *net.IPAddr {
	return &net.IPAddr{
		IP: ap.Addr.AsSlice(),
	}
}

func (ap Socksaddr) TCPAddr() *net.TCPAddr {
	return &net.TCPAddr{
		IP:   ap.Addr.AsSlice(),
		Port: int(ap.Port),
	}
}

func (ap Socksaddr) UDPAddr() *net.UDPAddr {
	return &net.UDPAddr{
		IP:   ap.Addr.AsSlice(),
		Port: int(ap.Port),
	}
}

func (ap Socksaddr) AddrPort() netip.AddrPort {
	return netip.AddrPortFrom(ap.Addr, ap.Port)
}

func (ap Socksaddr) String() string {
	return net.JoinHostPort(ap.AddrString(), strconv.Itoa(int(ap.Port)))
}

func TCPAddr(ap netip.AddrPort) *net.TCPAddr {
	return &net.TCPAddr{
		IP:   ap.Addr().AsSlice(),
		Port: int(ap.Port()),
	}
}

func UDPAddr(ap netip.AddrPort) *net.UDPAddr {
	return &net.UDPAddr{
		IP:   ap.Addr().AsSlice(),
		Port: int(ap.Port()),
	}
}

func AddrPortFrom(ip net.IP, port uint16) netip.AddrPort {
	addr, _ := netip.AddrFromSlice(ip)
	return netip.AddrPortFrom(addr, port)
}

func SocksaddrFrom(ip net.IP, port uint16) Socksaddr {
	return SocksaddrFromNetIP(AddrPortFrom(ip, port))
}

func SocksaddrFromAddrPort(addr netip.Addr, port uint16) Socksaddr {
	return SocksaddrFromNetIP(netip.AddrPortFrom(addr, port))
}

func SocksaddrFromNetIP(ap netip.AddrPort) Socksaddr {
	if ap.Addr().Is4In6() {
		return Socksaddr{
			Addr: netip.AddrFrom4(ap.Addr().As4()),
			Port: ap.Port(),
		}
	}
	return Socksaddr{
		Addr: ap.Addr(),
		Port: ap.Port(),
	}
}

func SocksaddrFromNet(ap net.Addr) Socksaddr {
	if socksAddr, ok := ap.(Socksaddr); ok {
		return socksAddr
	}
	return SocksaddrFromNetIP(AddrPortFromNet(ap))
}

func AddrFromNetAddr(netAddr net.Addr) netip.Addr {
	if addr := AddrPortFromNet(netAddr); addr.Addr().IsValid() {
		return addr.Addr()
	}
	switch addr := netAddr.(type) {
	case Socksaddr:
		return addr.Addr
	case *net.IPAddr:
		return AddrFromIP(addr.IP)
	case *net.IPNet:
		return AddrFromIP(addr.IP)
	default:
		return netip.Addr{}
	}
}

func AddrPortFromNet(netAddr net.Addr) netip.AddrPort {
	var ip net.IP
	var port uint16
	switch addr := netAddr.(type) {
	case Socksaddr:
		return addr.AddrPort()
	case *net.TCPAddr:
		ip = addr.IP
		port = uint16(addr.Port)
	case *net.UDPAddr:
		ip = addr.IP
		port = uint16(addr.Port)
	case *net.IPAddr:
		ip = addr.IP
	}
	return netip.AddrPortFrom(AddrFromIP(ip), port)
}

func AddrFromIP(ip net.IP) netip.Addr {
	addr, _ := netip.AddrFromSlice(ip)
	if addr.Is4In6() {
		addr = netip.AddrFrom4(addr.As4())
	}
	return addr
}

func ParseAddr(s string) netip.Addr {
	addr, _ := netip.ParseAddr(s)
	if addr.Is4In6() {
		addr = netip.AddrFrom4(addr.As4())
	}
	return addr
}

func ParseSocksaddr(address string) Socksaddr {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return Socksaddr{}
	}
	return ParseSocksaddrHostPort(host, port)
}

func ParseSocksaddrHostPort(host string, portStr string) Socksaddr {
	port, _ := strconv.Atoi(portStr)
	netAddr, err := netip.ParseAddr(host)
	if netAddr.Is4In6() {
		netAddr = netip.AddrFrom4(netAddr.As4())
	}
	if err != nil {
		return Socksaddr{
			Fqdn: host,
			Port: uint16(port),
		}
	} else {
		return Socksaddr{
			Addr: netAddr,
			Port: uint16(port),
		}
	}
}
