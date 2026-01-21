package network

// StreamPollable provides reactor support for TCP stream connections
// Used by StreamReactor for idle detection via epoll/kqueue/IOCP
type StreamPollable interface {
	// FD returns the file descriptor for reactor registration
	FD() int
	// Buffered returns the number of bytes in internal buffer
	// Reactor must check this before returning to idle state
	Buffered() int
}

// StreamPollableCreator creates a StreamPollable dynamically
// Optional interface - prefer direct implementation of StreamPollable
type StreamPollableCreator interface {
	CreateStreamPollable() (StreamPollable, bool)
}
