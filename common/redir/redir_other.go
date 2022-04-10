//go:build !linux

package redir

import (
	"errors"
	"net"

	M "github.com/sagernet/sing/common/metadata"
)

func GetOriginalDestination(conn net.Conn) (destination *M.AddrPort, err error) {
	return nil, errors.New("unsupported platform")
}
