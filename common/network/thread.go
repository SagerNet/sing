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

func IsSafeReader(reader any) ThreadSafeReader {
	if safeReader, isSafe := reader.(ThreadSafeReader); isSafe {
		return safeReader
	}
	if upstream, hasUpstream := reader.(ReaderWithUpstream); !hasUpstream || !upstream.ReaderReplaceable() {
		return nil
	}
	if upstream, hasUpstream := reader.(common.WithUpstream); hasUpstream {
		return IsSafeReader(upstream.Upstream())
	}
	if upstream, hasUpstream := reader.(WithUpstreamReader); hasUpstream {
		return IsSafeReader(upstream.UpstreamReader())
	}
	return nil
}

func IsSafePacketReader(reader any) ThreadSafePacketReader {
	if safeReader, isSafe := reader.(ThreadSafePacketReader); isSafe {
		return safeReader
	}
	if upstream, hasUpstream := reader.(ReaderWithUpstream); !hasUpstream || !upstream.ReaderReplaceable() {
		return nil
	}
	if upstream, hasUpstream := reader.(common.WithUpstream); hasUpstream {
		return IsSafePacketReader(upstream.Upstream())
	}
	if upstream, hasUpstream := reader.(WithUpstreamReader); hasUpstream {
		return IsSafePacketReader(upstream.UpstreamReader())
	}
	return nil
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
	if upstreamWriter, hasUpstreamWriter := writer.(WithUpstreamWriter); hasUpstreamWriter {
		headroom += CalculateFrontHeadroom(upstreamWriter.UpstreamWriter())
	} else if upstream, hasUpstream := writer.(common.WithUpstream); hasUpstream {
		headroom += CalculateFrontHeadroom(upstream.Upstream())
	}
	return headroom
}

func CalculateRearHeadroom(writer any) int {
	var headroom int
	if headroomWriter, needHeadroom := writer.(RearHeadroom); needHeadroom {
		headroom = headroomWriter.RearHeadroom()
	}
	if upstreamWriter, hasUpstreamWriter := writer.(WithUpstreamWriter); hasUpstreamWriter {
		headroom += CalculateRearHeadroom(upstreamWriter.UpstreamWriter())
	} else if upstream, hasUpstream := writer.(common.WithUpstream); hasUpstream {
		headroom += CalculateRearHeadroom(upstream.Upstream())
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
	readerMTU := calculateReaderMTU(reader)
	writerMTU := calculateWriterMTU(writer)
	if readerMTU > writerMTU {
		return readerMTU
	}
	if writerMTU > buf.BufferSize {
		return 0
	}
	return writerMTU
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
