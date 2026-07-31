//go:build windows

package winiphlpapi

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const SioTcpInfo = 0xD8000027

type TcpInfoV0 struct {
	State             uint32
	Mss               uint32
	ConnectionTimeMs  uint64
	TimestampsEnabled uint8
	RttUs             uint32
	MinRttUs          uint32
	BytesInFlight     uint32
	Cwnd              uint32
	SndWnd            uint32
	RcvWnd            uint32
	RcvBuf            uint32
	BytesOut          uint64
	BytesIn           uint64
	BytesReordered    uint32
	BytesRetrans      uint32
	FastRetrans       uint32
	DupAcksIn         uint32
	TimeoutEpisodes   uint32
	SynRetrans        uint8
}

func GetTcpInfo(fd uintptr) (*TcpInfoV0, error) {
	version := uint32(0)
	var tcpInfo TcpInfoV0
	var bytesReturned uint32
	err := windows.WSAIoctl(
		windows.Handle(fd),
		SioTcpInfo,
		(*byte)(unsafe.Pointer(&version)),
		uint32(unsafe.Sizeof(version)),
		(*byte)(unsafe.Pointer(&tcpInfo)),
		uint32(unsafe.Sizeof(tcpInfo)),
		&bytesReturned,
		nil,
		0,
	)
	if err != nil {
		return nil, err
	}
	return &tcpInfo, nil
}
