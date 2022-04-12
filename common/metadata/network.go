package metadata

import "net/netip"

func NetworkFromNetAddr(network string, addr netip.Addr) string {
	if addr.Is4() && (addr.IsUnspecified() || addr.IsGlobalUnicast() || addr.IsLinkLocalUnicast()) {
		return network + "4"
	}
	return network
}
