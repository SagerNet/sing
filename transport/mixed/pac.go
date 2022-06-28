package mixed

import (
	"net/netip"
)

func newPAC(proxyAddr netip.AddrPort) string {
	return `
function FindProxyForURL(url, host) {
    return "SOCKS5 ` + proxyAddr.String() + `; PROXY ` + proxyAddr.String() + `";
}`
}
