//go:build !linux

package redir

import (
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
)

func TProxy(fd uintptr, isIPv6 bool) error {
	return E.New("only available on linux")
}

func TProxyUDP(fd uintptr, isIPv6 bool) error {
	return E.New("only available on linux")
}

func GetOriginalDestinationFromOOB(oob []byte) (*M.AddrPort, error) {
	return nil, E.New("only available on linux")
}
