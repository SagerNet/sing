package network

// PacketPushable represents a packet source that receives pushed data
// from external code and notifies reactor via callback.
type PacketPushable interface {
	SetOnDataReady(callback func())
	HasPendingData() bool
}

// PacketPollable provides FD-based polling for packet connections.
// Mirrors StreamPollable for consistency.
type PacketPollable interface {
	FD() int
}

// PacketPollableCreator creates a PacketPollable dynamically.
type PacketPollableCreator interface {
	CreatePacketPollable() (PacketPollable, bool)
}
