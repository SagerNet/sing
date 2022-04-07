package common

import (
	"io"
)

type ReaderWithUpstream interface {
	Upstream() io.Reader
	Replaceable() bool
}

type UpstreamReaderSetter interface {
	SetUpstream(reader io.Reader)
}

type WriterWithUpstream interface {
	Upstream() io.Writer
	Replaceable() bool
}

type UpstreamWriterSetter interface {
	SetWriter(writer io.Writer)
}
