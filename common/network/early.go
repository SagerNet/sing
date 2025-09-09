package network

import (
	"io"

	"github.com/sagernet/sing/common"
)

// Deprecated: use EarlyReader and EarlyWriter instead.
type EarlyConn interface {
	NeedHandshake() bool
}

type EarlyReader interface {
	NeedHandshakeForRead() bool
}

func NeedHandshakeForRead(reader io.Reader) bool {
	if earlyReader, isEarlyReader := common.Cast[EarlyReader](reader); isEarlyReader && earlyReader.NeedHandshakeForRead() {
		return true
	}
	return false
}

type EarlyWriter interface {
	NeedHandshakeForWrite() bool
}

func NeedHandshakeForWrite(writer io.Writer) bool {
	if //goland:noinspection GoDeprecation
	earlyConn, isEarlyConn := writer.(EarlyConn); isEarlyConn {
		return earlyConn.NeedHandshake()
	}
	if earlyWriter, isEarlyWriter := common.Cast[EarlyWriter](writer); isEarlyWriter && earlyWriter.NeedHandshakeForWrite() {
		return true
	}
	return false
}
