package mixed

import (
	"net/netip"
)

/*func newPAC(proxyAddr M.Socksaddr) string {
	return `
function FindProxyForURL(url, host) {
    return "SOCKS5 ` + proxyAddr.String() + `;SOCKS ` + proxyAddr.String() + `; PROXY ` + proxyAddr.String() + `";
}`
}
*/

func newPAC(proxyAddr netip.AddrPort) string {
	// TODO: socks4 not supported
	return `
function FindProxyForURL(url, host) {
    return "SOCKS5 ` + proxyAddr.String() + `; PROXY ` + proxyAddr.String() + `";
}`
}
