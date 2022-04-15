package network

import (
	"net"
	"net/netip"

	"github.com/sagernet/sing/common"
	M "github.com/sagernet/sing/common/metadata"
)

func LocalAddrs() ([]netip.Addr, error) {
	interfaceAddrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	return common.Map(common.Filter(common.Map(interfaceAddrs, func(addr net.Addr) M.Addr {
		return M.AddrFromNetAddr(addr)
	}), func(addr M.Addr) bool {
		return addr != nil
	}), func(it M.Addr) netip.Addr {
		return it.Addr()
	}), nil
}

func LocalPublicAddrs() ([]netip.Addr, error) {
	publicAddrs, err := LocalAddrs()
	if err != nil {
		return nil, err
	}
	return common.Filter(publicAddrs, func(addr netip.Addr) bool {
		return !(addr.IsPrivate() || addr.IsLoopback() || addr.IsMulticast() || addr.IsGlobalUnicast() || addr.IsLinkLocalUnicast() || addr.IsInterfaceLocalMulticast())
	}), nil
}
