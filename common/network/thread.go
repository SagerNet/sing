package network

import (
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
)

type ThreadUnsafeWriter interface {
	WriteIsThreadUnsafe()
}

type ThreadSafeReader interface {
	ReadBufferThreadSafe() (buffer *buf.Buffer, err error)
}

type ThreadSafePacketReader interface {
	ReadPacketThreadSafe() (buffer *buf.Buffer, addr M.Socksaddr, err error)
}

type HeadroomWriter interface {
	Headroom() int
}

func IsUnsafeWriter(writer any) bool {
	_, isUnsafe := common.Cast[ThreadUnsafeWriter](writer)
	return isUnsafe
}

func CalculateHeadroom(writer any) int {
	var headroom int
	if headroomWriter, needHeadroom := writer.(HeadroomWriter); needHeadroom {
		headroom = headroomWriter.Headroom()
	}
	if upstream, hasUpstream := writer.(common.WithUpstream); hasUpstream {
		return headroom + CalculateHeadroom(upstream.Upstream())
	}
	return headroom
}
