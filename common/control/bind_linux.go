package control

import (
	"os"
	"syscall"

	"github.com/sagernet/sing/common/atomic"
	E "github.com/sagernet/sing/common/exceptions"

	"golang.org/x/sys/unix"
)

var ifIndexDisabled atomic.Bool

func bindToInterface(conn syscall.RawConn, network string, address string, finder InterfaceFinder, interfaceName string, interfaceIndex int, preferInterfaceName bool) error {
	return Raw(conn, func(fd uintptr) error {
		if interfaceIndex != -1 {
			return unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_BINDTOIFINDEX, interfaceIndex)
		}
		if interfaceName == "" {
			return os.ErrInvalid
		}
		if !preferInterfaceName && finder != nil && !ifIndexDisabled.Load() {
			var err error
			interfaceIndex, err = finder.InterfaceIndexByName(interfaceName)
			if err != nil {
				return err
			}
			err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_BINDTOIFINDEX, interfaceIndex)
			if err == nil {
				return nil
			} else if E.IsMulti(err, unix.ENOPROTOOPT, unix.EINVAL) {
				ifIndexDisabled.Store(true)
			} else {
				return err
			}
		}
		return unix.BindToDevice(int(fd), interfaceName)
	})
}
