package metadata

import (
	"encoding/binary"
	"io"

	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/rw"
)

type SerializerOption func(*Serializer)

func AddressFamilyByte(b byte, f Family) SerializerOption {
	return func(s *Serializer) {
		s.familyMap[b] = f
		s.familyByteMap[f] = b
	}
}

func PortThenAddress() SerializerOption {
	return func(s *Serializer) {
		s.portFirst = true
	}
}

type Serializer struct {
	familyMap     map[byte]Family
	familyByteMap map[Family]byte
	portFirst     bool
}

func NewSerializer(options ...SerializerOption) *Serializer {
	s := &Serializer{
		familyMap:     make(map[byte]Family),
		familyByteMap: make(map[Family]byte),
	}
	for _, option := range options {
		option(s)
	}
	return s
}

func (s *Serializer) WriteAddress(writer io.Writer, addr Addr) error {
	err := rw.WriteByte(writer, s.familyByteMap[addr.Family()])
	if err != nil {
		return err
	}
	if addr.Family().IsIP() {
		err = rw.WriteBytes(writer, addr.Addr().AsSlice())
	} else {
		domain := addr.Fqdn()
		err = WriteString(writer, "fqdn", domain)
	}
	return err
}

func (s *Serializer) AddressLen(addr Addr) int {
	switch addr.Family() {
	case AddressFamilyIPv4:
		return 5
	case AddressFamilyIPv6:
		return 17
	default:
		return 2 + len(addr.Fqdn())
	}
}

func (s *Serializer) WritePort(writer io.Writer, port uint16) error {
	return binary.Write(writer, binary.BigEndian, port)
}

func (s *Serializer) WriteAddrPort(writer io.Writer, destination *AddrPort) error {
	var err error
	if !s.portFirst {
		err = s.WriteAddress(writer, destination.Addr)
	} else {
		err = s.WritePort(writer, destination.Port)
	}
	if err != nil {
		return err
	}
	if s.portFirst {
		err = s.WriteAddress(writer, destination.Addr)
	} else {
		err = s.WritePort(writer, destination.Port)
	}
	return err
}

func (s *Serializer) AddrPortLen(destination *AddrPort) int {
	return s.AddressLen(destination.Addr) + 2
}

func (s *Serializer) ReadAddress(reader io.Reader) (Addr, error) {
	af, err := rw.ReadByte(reader)
	if err != nil {
		return nil, err
	}
	family := s.familyMap[af]
	switch family {
	case AddressFamilyFqdn:
		fqdn, err := ReadString(reader)
		if err != nil {
			return nil, E.Cause(err, "read fqdn")
		}
		return AddrFqdn(fqdn), nil
	default:
		switch family {
		case AddressFamilyIPv4:
			var addr [4]byte
			err = common.Error(reader.Read(addr[:]))
			if err != nil {
				return nil, E.Cause(err, "read ipv4 address")
			}
			return Addr4(addr), nil
		case AddressFamilyIPv6:
			var addr [16]byte
			err = common.Error(reader.Read(addr[:]))
			if err != nil {
				return nil, E.Cause(err, "read ipv6 address")
			}
			return Addr16(addr), nil
		default:
			return nil, E.New("unknown address family: ", af)
		}
	}
}

func (s *Serializer) ReadPort(reader io.Reader) (uint16, error) {
	port, err := rw.ReadBytes(reader, 2)
	if err != nil {
		return 0, E.Cause(err, "read port")
	}
	return binary.BigEndian.Uint16(port), nil
}

func (s *Serializer) ReadAddrPort(reader io.Reader) (destination *AddrPort, err error) {
	var addr Addr
	var port uint16
	if !s.portFirst {
		addr, err = s.ReadAddress(reader)
	} else {
		port, err = s.ReadPort(reader)
	}
	if err != nil {
		return
	}
	if s.portFirst {
		addr, err = s.ReadAddress(reader)
	} else {
		port, err = s.ReadPort(reader)
	}
	if err != nil {
		return
	}
	return AddrPortFrom(addr, port), nil
}

func ReadString(reader io.Reader) (string, error) {
	strLen, err := rw.ReadByte(reader)
	if err != nil {
		return common.EmptyString, err
	}
	return rw.ReadString(reader, int(strLen))
}

func WriteString(writer io.Writer, op string, str string) error {
	strLen := len(str)
	if strLen > 255 {
		return &StringTooLongException{op, strLen}
	}
	err := rw.WriteByte(writer, byte(strLen))
	if err != nil {
		return err
	}
	return rw.WriteString(writer, str)
}
