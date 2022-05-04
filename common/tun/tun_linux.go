package tun

/*
import (
	"bytes"
	"net"
	"syscall"
	"unsafe"

	E "github.com/sagernet/sing/common/exceptions"
	"golang.org/x/sys/unix"
)

const ifReqSize = unix.IFNAMSIZ + 64

func (t *Interface) Name() (string, error) {
	if t.tunName != "" {
		return t.tunName, nil
	}
	var ifr [ifReqSize]byte
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(t.tunFd), uintptr(unix.TUNGETIFF), uintptr(unsafe.Pointer(&ifr[0])))
	if errno != 0 {
		return "", errno
	}
	name := ifr[:]
	if i := bytes.IndexByte(name, 0); i != -1 {
		name = name[:i]
	}
	t.tunName = string(name)
	return t.tunName, nil
}

func (t *Interface) MTU() (int, error) {
	name, err := t.Name()
	if err != nil {
		return 0, err
	}
	fd, err := unix.Socket(
		unix.AF_INET,
		unix.SOCK_DGRAM,
		0,
	)
	if err != nil {
		return 0, err
	}
	defer unix.Close(fd)
	var ifr [ifReqSize]byte
	copy(ifr[:], name)
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(unix.SIOCGIFMTU), uintptr(unsafe.Pointer(&ifr[0])))
	if errno != 0 {
		return 0, errno
	}
	return int(*(*int32)(unsafe.Pointer(&ifr[unix.IFNAMSIZ]))), nil
}

func (t *Interface) SetMTU(mtu int) error {
	name, err := t.Name()
	if err != nil {
		return err
	}
	fd, err := unix.Socket(
		unix.AF_INET,
		unix.SOCK_DGRAM,
		0,
	)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var ifr [ifReqSize]byte
	copy(ifr[:], name)
	*(*uint32)(unsafe.Pointer(&ifr[unix.IFNAMSIZ])) = uint32(mtu)
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(fd),
		uintptr(unix.SIOCSIFMTU),
		uintptr(unsafe.Pointer(&ifr[0])),
	)
	if errno != 0 {
		return errno
	}
	return nil
}

func (t *Interface) SetAddress() error {
	name, err := t.Name()
	if err != nil {
		return err
	}
	fd, err := unix.Socket(
		unix.AF_INET,
		unix.SOCK_DGRAM,
		0,
	)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	ifreq, err := unix.NewIfreq(name)
	if err != nil {
		return E.Cause(err, "failed to create ifreq for name ", name)
	}

	ifreq.SetInet4Addr(t.inetAddress.Addr().AsSlice())
	err = unix.IoctlIfreq(fd, syscall.SIOCSIFADDR, ifreq)
	if err == nil {
		ifreq, _ = unix.NewIfreq(name)
		ifreq.SetInet4Addr(net.CIDRMask(t.inetAddress.Bits(), 32))
		err = unix.IoctlIfreq(fd, syscall.SIOCSIFNETMASK, ifreq)
	}
	if err != nil {
		return E.Cause(err, "failed to set ipv4 address on ", name)
	}
	if t.inet6Address.IsValid() {
		ifreq, _ = unix.NewIfreq(name)
		err = unix.IoctlIfreq(fd, syscall.SIOCGIFINDEX, ifreq)
		if err != nil {
			return E.Cause(err, "failed to get interface index for ", name)
		}

		ifreq6 := in6_ifreq{
			ifr6_addr: in6_addr{
				addr: t.inet6Address.Addr().As16(),
			},
			ifr6_prefixlen: uint32(t.inet6Address.Bits()),
			ifr6_ifindex:   ifreq.Uint32(),
		}

		fd6, err := unix.Socket(
			unix.AF_INET6,
			unix.SOCK_DGRAM,
			0,
		)
		if err != nil {
			return err
		}
		defer unix.Close(fd6)

		if _, _, errno := syscall.Syscall(
			syscall.SYS_IOCTL,
			uintptr(fd6),
			uintptr(syscall.SIOCSIFADDR),
			uintptr(unsafe.Pointer(&ifreq6)),
		); errno != 0 {
			return E.Cause(errno, "failed to set ipv6 address on ", name)
		}
	}

	ifreq, _ = unix.NewIfreq(name)
	err = unix.IoctlIfreq(fd, syscall.SIOCGIFFLAGS, ifreq)
	if err == nil {
		ifreq.SetUint16(ifreq.Uint16() | syscall.IFF_UP | syscall.IFF_RUNNING)
		err = unix.IoctlIfreq(fd, syscall.SIOCSIFFLAGS, ifreq)
	}
	if err != nil {
		return E.Cause(err, "failed to bring tun device up")
	}

	return nil
}

type in6_addr struct {
	addr [16]byte
}

type in6_ifreq struct {
	ifr6_addr      in6_addr
	ifr6_prefixlen uint32
	ifr6_ifindex   uint32
}
*/
