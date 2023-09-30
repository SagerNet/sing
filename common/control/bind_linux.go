package control

import (
	"os"
	"syscall"

	"github.com/sagernet/sing/common/atomic"
	E "github.com/sagernet/sing/common/exceptions"

	"golang.org/x/sys/unix"
)

var ifIndexDisabled atomic.Bool

func bindToInterface(conn syscall.RawConn, network string, address string, finder InterfaceFinder, interfaceName string, interfaceIndex int) error {
	return Raw(conn, func(fd uintptr) error {
		var err error
		if !ifIndexDisabled.Load() {
			if interfaceIndex == -1 {
				if finder == nil {
					return os.ErrInvalid
				}
				interfaceIndex, err = finder.InterfaceIndexByName(interfaceName)
				if err != nil {
					return err
				}
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
		if interfaceName == "" {
			if finder == nil {
				return os.ErrInvalid
			}
			interfaceName, err = finder.InterfaceNameByIndex(interfaceIndex)
			if err != nil {
				return err
			}
		}
		return unix.BindToDevice(int(fd), interfaceName)
	})
}
