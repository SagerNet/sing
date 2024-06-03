package http

import (
	"net/http"
	"strings"

	M "github.com/sagernet/sing/common/metadata"
)

func SourceAddress(request *http.Request) M.Socksaddr {
	address := M.ParseSocksaddr(request.RemoteAddr)
	forwardFrom := request.Header.Get("X-Forwarded-For")
	if forwardFrom != "" {
		// Get the leftmost IP, which is the client's original IP address
		ips := strings.Split(forwardFrom, ",")
		// Trim any leading or trailing spaces from the IP address
		originalClientIP := strings.TrimSpace(ips[0])
		originAddr := M.ParseAddr(originalClientIP)
		if originAddr.IsValid() {
			address.Addr = originAddr
		}
	}
	return address.Unwrap()
}
