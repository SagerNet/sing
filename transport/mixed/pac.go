package mixed

import M "github.com/sagernet/sing/common/metadata"

/*func newPAC(proxyAddr *M.AddrPort) string {
	return `
function FindProxyForURL(url, host) {
    return "SOCKS5 ` + proxyAddr.String() + `;SOCKS ` + proxyAddr.String() + `; PROXY ` + proxyAddr.String() + `";
}`
}
*/

func newPAC(proxyAddr *M.AddrPort) string {
	// TODO: socks4 not supported
	return `
function FindProxyForURL(url, host) {
    return "SOCKS5 ` + proxyAddr.String() + `; PROXY ` + proxyAddr.String() + `";
}`
}
