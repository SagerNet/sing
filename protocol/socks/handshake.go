package socks

import (
	"io"

	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
)

func ClientHandshake(conn io.ReadWriter, version byte, command byte, destination *M.AddrPort, username string, password string) (*Response, error) {
	var method byte
	if common.IsBlank(username) {
		method = AuthTypeNotRequired
	} else {
		method = AuthTypeUsernamePassword
	}
	err := WriteAuthRequest(conn, &AuthRequest{
		Version: version,
		Methods: []byte{method},
	})
	if err != nil {
		return nil, err
	}
	authResponse, err := ReadAuthResponse(conn)
	if err != nil {
		return nil, err
	}
	if authResponse.Method != method {
		return nil, E.New("not requested method, request ", method, ", return ", method)
	}
	if method == AuthTypeUsernamePassword {
		err = WriteUsernamePasswordAuthRequest(conn, &UsernamePasswordAuthRequest{
			Username: username,
			Password: password,
		})
		if err != nil {
			return nil, err
		}
		usernamePasswordResponse, err := ReadUsernamePasswordAuthResponse(conn)
		if err != nil {
			return nil, err
		}
		if usernamePasswordResponse.Status == UsernamePasswordStatusFailure {
			return nil, &UsernamePasswordAuthFailureException{}
		}
	}
	err = WriteRequest(conn, &Request{
		Version:     version,
		Command:     command,
		Destination: destination,
	})
	if err != nil {
		return nil, err
	}
	return ReadResponse(conn)
}

func ClientFastHandshake(writer io.Writer, version byte, command byte, destination *M.AddrPort, username string, password string) error {
	var method byte
	if common.IsBlank(username) {
		method = AuthTypeNotRequired
	} else {
		method = AuthTypeUsernamePassword
	}
	err := WriteAuthRequest(writer, &AuthRequest{
		Version: version,
		Methods: []byte{method},
	})
	if err != nil {
		return err
	}
	if method == AuthTypeUsernamePassword {
		err = WriteUsernamePasswordAuthRequest(writer, &UsernamePasswordAuthRequest{
			Username: username,
			Password: password,
		})
		if err != nil {
			return err
		}
	}
	return WriteRequest(writer, &Request{
		Version:     version,
		Command:     command,
		Destination: destination,
	})
}

func ClientFastHandshakeFinish(reader io.Reader) (*Response, error) {
	response, err := ReadAuthResponse(reader)
	if err != nil {
		return nil, err
	}
	if response.Method == AuthTypeUsernamePassword {
		usernamePasswordResponse, err := ReadUsernamePasswordAuthResponse(reader)
		if err != nil {
			return nil, err
		}
		if usernamePasswordResponse.Status == UsernamePasswordStatusFailure {
			return nil, &UsernamePasswordAuthFailureException{}
		}
	}
	return ReadResponse(reader)
}
