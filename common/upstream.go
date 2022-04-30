package common

import (
	"io"
)

type ReaderWithUpstream interface {
	UpstreamReader() io.Reader
	ReaderReplaceable() bool
}

type UpstreamReaderSetter interface {
	SetReader(reader io.Reader)
}

type WriterWithUpstream interface {
	UpstreamWriter() io.Writer
	WriterReplaceable() bool
}

type UpstreamWriterSetter interface {
	SetWriter(writer io.Writer)
}
