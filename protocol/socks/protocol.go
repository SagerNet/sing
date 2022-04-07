package socks

import (
	"bytes"
	"io"
	"net"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/rw"
	"github.com/sagernet/sing/common/socksaddr"
)

//+----+----------+----------+
//|VER | NMETHODS | METHODS  |
//+----+----------+----------+
//| 1  |    1     | 1 to 255 |
//+----+----------+----------+

type AuthRequest struct {
	Version byte
	Methods []byte
}

func WriteAuthRequest(writer io.Writer, request *AuthRequest) error {
	err := rw.WriteByte(writer, request.Version)
	if err != nil {
		return err
	}
	err = rw.WriteByte(writer, byte(len(request.Methods)))
	if err != nil {
		return err
	}
	return rw.WriteBytes(writer, request.Methods)
}

func ReadAuthRequest(reader io.Reader) (*AuthRequest, error) {
	version, err := rw.ReadByte(reader)
	if err != nil {
		return nil, err
	}
	if version != Version5 {
		return nil, &UnsupportedVersionException{version}
	}
	methodLen, err := rw.ReadByte(reader)
	if err != nil {
		return nil, exceptions.Cause(err, "read socks auth methods length")
	}
	methods, err := rw.ReadBytes(reader, int(methodLen))
	if err != nil {
		return nil, exceptions.CauseF(err, "read socks auth methods, length ", methodLen)
	}
	request := &AuthRequest{
		version,
		methods,
	}
	return request, nil
}

//+----+--------+
//|VER | METHOD |
//+----+--------+
//| 1  |   1    |
//+----+--------+

type AuthResponse struct {
	Version byte
	Method  byte
}

func WriteAuthResponse(writer io.Writer, response *AuthResponse) error {
	err := rw.WriteByte(writer, response.Version)
	if err != nil {
		return err
	}
	return rw.WriteByte(writer, response.Method)
}

func ReadAuthResponse(reader io.Reader) (*AuthResponse, error) {
	version, err := rw.ReadByte(reader)
	if err != nil {
		return nil, err
	}
	if version != Version5 {
		return nil, &UnsupportedVersionException{version}
	}
	method, err := rw.ReadByte(reader)
	if err != nil {
		return nil, err
	}
	response := &AuthResponse{
		Version: version,
		Method:  method,
	}
	return response, nil
}

//+----+------+----------+------+----------+
//|VER | ULEN |  UNAME   | PLEN |  PASSWD  |
//+----+------+----------+------+----------+
//| 1  |  1   | 1 to 255 |  1   | 1 to 255 |
//+----+------+----------+------+----------+

type UsernamePasswordAuthRequest struct {
	Username string
	Password string
}

func WriteUsernamePasswordAuthRequest(writer io.Writer, request *UsernamePasswordAuthRequest) error {
	err := rw.WriteByte(writer, UsernamePasswordVersion1)
	if err != nil {
		return err
	}
	err = socksaddr.WriteString(writer, "username", request.Username)
	if err != nil {
		return err
	}
	return socksaddr.WriteString(writer, "password", request.Password)
}

func ReadUsernamePasswordAuthRequest(reader io.Reader) (*UsernamePasswordAuthRequest, error) {
	version, err := rw.ReadByte(reader)
	if err != nil {
		return nil, err
	}
	if version != UsernamePasswordVersion1 {
		return nil, &UnsupportedVersionException{version}
	}
	username, err := socksaddr.ReadString(reader)
	if err != nil {
		return nil, err
	}
	password, err := socksaddr.ReadString(reader)
	if err != nil {
		return nil, err
	}
	request := &UsernamePasswordAuthRequest{
		Username: username,
		Password: password,
	}
	return request, nil
}

//+----+--------+
//|VER | STATUS |
//+----+--------+
//| 1  |   1    |
//+----+--------+

type UsernamePasswordAuthResponse struct {
	Status byte
}

func WriteUsernamePasswordAuthResponse(writer io.Writer, response *UsernamePasswordAuthResponse) error {
	err := rw.WriteByte(writer, UsernamePasswordVersion1)
	if err != nil {
		return err
	}
	return rw.WriteByte(writer, response.Status)
}

func ReadUsernamePasswordAuthResponse(reader io.Reader) (*UsernamePasswordAuthResponse, error) {
	version, err := rw.ReadByte(reader)
	if err != nil {
		return nil, err
	}
	if version != UsernamePasswordVersion1 {
		return nil, &UnsupportedVersionException{version}
	}
	status, err := rw.ReadByte(reader)
	if status != UsernamePasswordStatusSuccess {
		status = UsernamePasswordStatusFailure
	}
	response := &UsernamePasswordAuthResponse{
		Status: status,
	}
	return response, nil
}

//+----+-----+-------+------+----------+----------+
//|VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
//+----+-----+-------+------+----------+----------+
//| 1  |  1  | X'00' |  1   | Variable |    2     |
//+----+-----+-------+------+----------+----------+

type Request struct {
	Version byte
	Command byte
	Addr    socksaddr.Addr
	Port    uint16
}

func WriteRequest(writer io.Writer, request *Request) error {
	err := rw.WriteByte(writer, request.Version)
	if err != nil {
		return err
	}
	err = rw.WriteByte(writer, request.Command)
	if err != nil {
		return err
	}
	err = rw.WriteZero(writer)
	if err != nil {
		return err
	}
	return AddressSerializer.WriteAddressAndPort(writer, request.Addr, request.Port)
}

func ReadRequest(reader io.Reader) (*Request, error) {
	version, err := rw.ReadByte(reader)
	if err != nil {
		return nil, err
	}
	if !(version == Version4 || version == Version5) {
		return nil, &UnsupportedVersionException{version}
	}
	command, err := rw.ReadByte(reader)
	if err != nil {
		return nil, err
	}
	if command != CommandConnect && command != CommandUDPAssociate {
		return nil, &UnsupportedCommandException{command}
	}
	err = rw.Skip(reader)
	if err != nil {
		return nil, err
	}
	addr, port, err := AddressSerializer.ReadAddressAndPort(reader)
	if err != nil {
		return nil, err
	}
	request := &Request{
		Version: version,
		Command: command,
		Addr:    addr,
		Port:    port,
	}
	return request, nil
}

//+----+-----+-------+------+----------+----------+
//|VER | REP |  RSV  | ATYP | BND.ADDR | BND.PORT |
//+----+-----+-------+------+----------+----------+
//| 1  |  1  | X'00' |  1   | Variable |    2     |
//+----+-----+-------+------+----------+----------+

type Response struct {
	Version   byte
	ReplyCode ReplyCode
	BindAddr  socksaddr.Addr
	BindPort  uint16
}

func WriteResponse(writer io.Writer, response *Response) error {
	err := rw.WriteByte(writer, response.Version)
	if err != nil {
		return err
	}
	err = rw.WriteByte(writer, byte(response.ReplyCode))
	if err != nil {
		return err
	}
	err = rw.WriteZero(writer)
	if err != nil {
		return err
	}
	if response.BindAddr == nil {
		return AddressSerializer.WriteAddressAndPort(writer, socksaddr.AddrFromIP(net.IPv4zero), response.BindPort)
	}
	return AddressSerializer.WriteAddressAndPort(writer, response.BindAddr, response.BindPort)
}

func ReadResponse(reader io.Reader) (*Response, error) {
	version, err := rw.ReadByte(reader)
	if err != nil {
		return nil, err
	}
	if !(version == Version4 || version == Version5) {
		return nil, &UnsupportedVersionException{version}
	}
	replyCode, err := rw.ReadByte(reader)
	if err != nil {
		return nil, err
	}
	err = rw.Skip(reader)
	if err != nil {
		return nil, err
	}
	addr, port, err := AddressSerializer.ReadAddressAndPort(reader)
	if err != nil {
		return nil, err
	}
	response := &Response{
		Version:   version,
		ReplyCode: ReplyCode(replyCode),
		BindAddr:  addr,
		BindPort:  port,
	}
	return response, nil
}

//+----+------+------+----------+----------+----------+
//|RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
//+----+------+------+----------+----------+----------+
//| 2  |  1   |  1   | Variable |    2     | Variable |
//+----+------+------+----------+----------+----------+

type AssociatePacket struct {
	Fragment byte
	Addr     socksaddr.Addr
	Port     uint16
	Data     []byte
}

func DecodeAssociatePacket(buffer *buf.Buffer) (*AssociatePacket, error) {
	if buffer.Len() < 5 {
		return nil, exceptions.New("insufficient length")
	}
	fragment := buffer.Byte(2)
	reader := bytes.NewReader(buffer.Bytes())
	err := common.Error(reader.Seek(3, io.SeekStart))
	if err != nil {
		return nil, err
	}
	addr, port, err := AddressSerializer.ReadAddressAndPort(reader)
	if err != nil {
		return nil, err
	}
	buffer.Advance(reader.Len())
	packet := &AssociatePacket{
		Fragment: fragment,
		Addr:     addr,
		Port:     port,
		Data:     buffer.Bytes(),
	}
	return packet, nil
}

func EncodeAssociatePacket(packet *AssociatePacket, buffer *buf.Buffer) error {
	err := rw.WriteZeroN(buffer, 2)
	if err != nil {
		return err
	}
	err = rw.WriteByte(buffer, packet.Fragment)
	if err != nil {
		return err
	}
	err = AddressSerializer.WriteAddressAndPort(buffer, packet.Addr, packet.Port)
	if err != nil {
		return err
	}
	_, err = buffer.Write(packet.Data)
	return err
}
