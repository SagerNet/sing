//go:build !linux

package redir

import (
	"errors"
	"net"
	"net/netip"
)

func GetOriginalDestination(conn net.Conn) (destination netip.AddrPort, err error) {
	return netip.AddrPort{}, errors.New("unsupported platform")
}
