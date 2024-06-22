package network

import (
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
)

type HandshakeFailure interface {
	HandshakeFailure(err error) error
}

type HandshakeSuccess interface {
	HandshakeSuccess() error
}

func ReportHandshakeFailure(conn any, err error) error {
	if handshakeConn, isHandshakeConn := common.Cast[HandshakeFailure](conn); isHandshakeConn {
		return E.Append(err, handshakeConn.HandshakeFailure(err), func(err error) error {
			return E.Cause(err, "write handshake failure")
		})
	}
	return err
}

func ReportHandshakeSuccess(conn any) error {
	if handshakeConn, isHandshakeConn := common.Cast[HandshakeSuccess](conn); isHandshakeConn {
		return handshakeConn.HandshakeSuccess()
	}
	return nil
}
