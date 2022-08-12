package metadata

import (
	"golang.org/x/sys/windows"
)

func (ap Socksaddr) Sockaddr() windows.Sockaddr {
	if ap.IsFqdn() {
		panic("bad sockaddr")
	} else if ap.IsIPv4() {
		return &windows.SockaddrInet4{
			Port: int(ap.Port),
			Addr: ap.Addr.As4(),
		}
	} else {
		return &windows.SockaddrInet6{
			Port: int(ap.Port),
			Addr: ap.Addr.As16(),
		}
	}
}
