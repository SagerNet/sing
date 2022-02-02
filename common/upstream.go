package common

import (
	"io"
)

type ReaderWithUpstream interface {
	Upstream() io.Reader
}

type WriterWithUpstream interface {
	Upstream() io.Writer
}
