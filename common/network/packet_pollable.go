package network

type PacketPollMode int

const (
	PacketPollModeChannel PacketPollMode = iota
	PacketPollModeFD
)

// PacketPollable provides polling support for packet connections
type PacketPollable interface {
	PollMode() PacketPollMode
	PacketChannel() <-chan *PacketBuffer
	FD() int
}

// PacketPollableCreator creates a PacketPollable dynamically
type PacketPollableCreator interface {
	CreatePacketPollable() (PacketPollable, bool)
}
