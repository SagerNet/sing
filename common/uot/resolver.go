package uot

import (
	"context"
	"net"
	"time"
)

var LookupAddress func(domain string) (net.IP, error)

func init() {
	LookupAddress = func(domain string) (net.IP, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", domain)
		cancel()
		if err != nil {
			return nil, err
		}
		return ips[0], nil
	}
}
