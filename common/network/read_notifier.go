package network

type ReadNotifier interface {
	isReadNotifier()
}

type ChannelNotifier struct {
	Channel <-chan *PacketBuffer
}

func (*ChannelNotifier) isReadNotifier() {}

type FileDescriptorNotifier struct {
	FD int
}

func (*FileDescriptorNotifier) isReadNotifier() {}

type ReadNotifierSource interface {
	CreateReadNotifier() ReadNotifier
}
