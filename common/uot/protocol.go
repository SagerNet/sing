package uot

import (
	"encoding/binary"
	"io"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
)

const (
	Version            = 2
	MagicAddress       = "sp.v2.udp-over-tcp.arpa"
	LegacyMagicAddress = "sp.udp-over-tcp.arpa"
)

var AddrParser = M.NewSerializer(
	M.AddressFamilyByte(0x00, M.AddressFamilyIPv4),
	M.AddressFamilyByte(0x01, M.AddressFamilyIPv6),
	M.AddressFamilyByte(0x02, M.AddressFamilyFqdn),
)

type Request struct {
	IsConnect   bool
	Destination M.Socksaddr
}

func ReadRequest(reader io.Reader) (*Request, error) {
	var version uint8
	err := binary.Read(reader, binary.BigEndian, &version)
	if err != nil {
		return nil, err
	}
	if version != Version {
		return nil, E.New("unsupported version: ", version)
	}
	var request Request
	err = binary.Read(reader, binary.BigEndian, &request.IsConnect)
	if err != nil {
		return nil, err
	}
	request.Destination, err = M.SocksaddrSerializer.ReadAddrPort(reader)
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func WriteRequest(writer io.Writer, request Request) error {
	var requestLen int
	requestLen += 1 // version
	requestLen += 1 // isConnect
	requestLen += M.SocksaddrSerializer.AddrPortLen(request.Destination)
	_buffer := buf.StackNewSize(requestLen)
	defer common.KeepAlive(_buffer)
	buffer := common.Dup(_buffer)
	defer buffer.Release()
	common.Must(
		binary.Write(buffer, binary.BigEndian, uint8(Version)),
		binary.Write(buffer, binary.BigEndian, request.IsConnect),
		M.SocksaddrSerializer.WriteAddrPort(buffer, request.Destination),
	)
	return common.Error(writer.Write(buffer.Bytes()))
}
