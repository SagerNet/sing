package network

type ThreadUnsafeWriter interface {
	WriteIsThreadUnsafe()
}
