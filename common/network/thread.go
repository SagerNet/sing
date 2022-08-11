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

func IsUnsafeWriter(writer any) bool {
	_, isUnsafe := common.Cast[ThreadUnsafeWriter](writer)
	return isUnsafe
}

type FrontHeadroom interface {
	FrontHeadroom() int
}

type RearHeadroom interface {
	RearHeadroom() int
}

func CalculateFrontHeadroom(writer any) int {
	var headroom int
	if headroomWriter, needHeadroom := writer.(FrontHeadroom); needHeadroom {
		headroom = headroomWriter.FrontHeadroom()
	}
	if upstream, hasUpstream := writer.(common.WithUpstream); hasUpstream {
		headroom += CalculateFrontHeadroom(upstream.Upstream())
	}
	if upstream, hasUpstream := writer.(WithUpstreamWriter); hasUpstream {
		headroom += CalculateFrontHeadroom(upstream.UpstreamWriter())
	}
	return headroom
}

func CalculateRearHeadroom(writer any) int {
	var headroom int
	if headroomWriter, needHeadroom := writer.(RearHeadroom); needHeadroom {
		headroom = headroomWriter.RearHeadroom()
	}
	if upstream, hasUpstream := writer.(common.WithUpstream); hasUpstream {
		headroom += CalculateRearHeadroom(upstream.Upstream())
	}

	if upstream, hasUpstream := writer.(WithUpstreamWriter); hasUpstream {
		headroom += CalculateRearHeadroom(upstream.UpstreamWriter())
	}
	return headroom
}

type ReaderWithMTU interface {
	ReaderMTU() int
}

type WriterWithMTU interface {
	WriterMTU() int
}

func CalculateMTU(reader any, writer any) int {
	mtu := calculateReaderMTU(reader)
	if mtu == 0 {
		return mtu
	}
	return calculateWriterMTU(writer)
}

func calculateReaderMTU(reader any) int {
	var mtu int
	if withMTU, haveMTU := reader.(ReaderWithMTU); haveMTU {
		mtu = withMTU.ReaderMTU()
	}
	if upstream, hasUpstream := reader.(common.WithUpstream); hasUpstream {
		upstreamMTU := calculateReaderMTU(upstream.Upstream())
		if upstreamMTU > mtu {
			mtu = upstreamMTU
		}
	}
	if upstream, hasUpstream := reader.(WithUpstreamReader); hasUpstream {
		upstreamMTU := calculateReaderMTU(upstream.UpstreamReader())
		if upstreamMTU > mtu {
			mtu = upstreamMTU
		}
	}
	return mtu
}

func calculateWriterMTU(writer any) int {
	var mtu int
	if withMTU, haveMTU := writer.(WriterWithMTU); haveMTU {
		mtu = withMTU.WriterMTU()
	}
	if upstream, hasUpstream := writer.(common.WithUpstream); hasUpstream {
		upstreamMTU := calculateWriterMTU(upstream.Upstream())
		if mtu == 0 && upstreamMTU < mtu {
			mtu = upstreamMTU
		}
	}
	if upstream, hasUpstream := writer.(WithUpstreamWriter); hasUpstream {
		upstreamMTU := calculateWriterMTU(upstream.UpstreamWriter())
		if mtu == 0 && upstreamMTU < mtu {
			mtu = upstreamMTU
		}
	}
	return mtu
}
