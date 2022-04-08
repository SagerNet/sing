package redir

import (
	"net"
	"net/netip"
	"syscall"
)

func GetOriginalDestination(conn net.Conn) (destination netip.AddrPort, err error) {
	rawConn, err := conn.(syscall.Conn).SyscallConn()
	if err != nil {
		return
	}
	var rawFd uintptr
	err = rawConn.Control(func(fd uintptr) {
		rawFd = fd
	})
	if err != nil {
		return
	}
	const SO_ORIGINAL_DST = 80
	if conn.RemoteAddr().(*net.TCPAddr).IP.To4() != nil {
		raw, err := syscall.GetsockoptIPv6Mreq(int(rawFd), syscall.IPPROTO_IP, SO_ORIGINAL_DST)
		if err != nil {
			return netip.AddrPort{}, err
		}
		addr, _ := netip.AddrFromSlice(raw.Multiaddr[4:8])
		return netip.AddrPortFrom(addr, uint16(raw.Multiaddr[2])<<8+uint16(raw.Multiaddr[3])), nil

	}
	raw, err := syscall.GetsockoptIPv6MTUInfo(int(rawFd), syscall.IPPROTO_IPV6, SO_ORIGINAL_DST)
	if err != nil {
		return
	}
	return netip.AddrPortFrom(netip.AddrFrom16(raw.Addr.Addr), raw.Addr.Port), nil
}
