package bufio

// FDHandler is the interface for handling FD ready events
// Implemented by both reactorStream (UDP) and streamDirection (TCP)
type FDHandler interface {
	// HandleFDEvent is called when the FD has data ready to read
	// The handler should start processing data in a new goroutine
	HandleFDEvent()
}
