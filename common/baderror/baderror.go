package baderror

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
)

func Contains(err error, msgList ...string) bool {
	for _, msg := range msgList {
		if strings.Contains(err.Error(), msg) {
			return true
		}
	}
	return false
}

// x/net http2 (clientConnReadLoop.run, serverConn.processFrameFromReader) and grpc-go
// (http2Client.reader, http2Server.HandleStreams) type-assert err.(http2.StreamError) on every
// error their Framer returns and keep reading instead of closing the connection, so an HTTP/2
// stream error leaking through an inner tunnel's Read spins the outer reader forever.
type protocolError struct {
	error
}

func (e *protocolError) Unwrap() error {
	return e.error
}

func WrapH2(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return io.EOF
	}
	if Contains(
		err,
		"client disconnected",
		"body closed by handler",
		"response body closed",
		"; CANCEL",
		"; NO_ERROR",
	) {
		return net.ErrClosed
	}
	if Contains(err, "stream error:", "connection error:", "http2: server sent GOAWAY") {
		return &protocolError{err}
	}
	return err
}

func WrapGRPC(err error) error {
	// grpc uses stupid internal error types
	if err == nil {
		return nil
	}
	if Contains(err, "EOF") {
		return io.EOF
	}
	if Contains(err, "Canceled") {
		return context.Canceled
	}
	if Contains(err,
		"the client connection is closing",
		"server closed the stream without sending trailers") {
		return net.ErrClosed
	}
	return err
}

// Deprecated: use qtls.WrapError
func WrapQUIC(err error) error {
	if err == nil {
		return nil
	}
	if Contains(err,
		"canceled by remote with error code 0",
		"canceled by local with error code 0",
	) {
		return net.ErrClosed
	}
	return err
}
