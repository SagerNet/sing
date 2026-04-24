package network

import (
	"errors"
	"io"

	"github.com/sagernet/sing/common"
)

var ErrHandshakeCompleted = errors.New("protocol handshake completed")

// Deprecated: use EarlyReader and EarlyWriter instead.
type EarlyConn interface {
	NeedHandshake() bool
}

type EarlyReader interface {
	NeedHandshakeForRead() bool
}

func NeedHandshakeForRead(reader io.Reader) bool {
	return NeedHandshakeForReadAny(reader)
}

func NeedHandshakeForReadAny(reader any) bool {
	if earlyReader, isEarlyReader := common.Cast[EarlyReader](reader); isEarlyReader && earlyReader.NeedHandshakeForRead() {
		return true
	}
	return false
}

type EarlyWriter interface {
	NeedHandshakeForWrite() bool
}

func NeedHandshakeForWrite(writer io.Writer) bool {
	return NeedHandshakeForWriteAny(writer)
}

func NeedHandshakeForWriteAny(writer any) bool {
	if //goland:noinspection GoDeprecation
	earlyConn, isEarlyConn := writer.(EarlyConn); isEarlyConn {
		return earlyConn.NeedHandshake()
	}
	if earlyWriter, isEarlyWriter := common.Cast[EarlyWriter](writer); isEarlyWriter && earlyWriter.NeedHandshakeForWrite() {
		return true
	}
	return false
}

type HandshakeState struct {
	readPending  bool
	writePending bool
	source       any
	destination  any
}

func NewHandshakeState(source io.Reader, destination io.Writer) HandshakeState {
	return HandshakeState{
		readPending:  NeedHandshakeForReadAny(source),
		writePending: NeedHandshakeForWriteAny(destination),
		source:       source,
		destination:  destination,
	}
}

func NewPacketHandshakeState(source PacketReader, destination PacketWriter) HandshakeState {
	return HandshakeState{
		readPending:  NeedHandshakeForReadAny(source),
		writePending: NeedHandshakeForWriteAny(destination),
		source:       source,
		destination:  destination,
	}
}

func (s HandshakeState) Upgradable() bool {
	return s.readPending || s.writePending
}

func (s HandshakeState) Check() error {
	if s.readPending && !NeedHandshakeForReadAny(s.source) {
		return ErrHandshakeCompleted
	}
	if s.writePending && !NeedHandshakeForWriteAny(s.destination) {
		return ErrHandshakeCompleted
	}
	return nil
}
