//go:build !linux && !darwin && !windows

package bufio

import (
	"context"

	E "github.com/sagernet/sing/common/exceptions"
)

type FDDemultiplexer struct{}

func NewFDDemultiplexer(ctx context.Context) (*FDDemultiplexer, error) {
	return nil, E.New("FDDemultiplexer not supported on this platform")
}

func (d *FDDemultiplexer) Add(stream *reactorStream, fd int) error {
	return E.New("FDDemultiplexer not supported on this platform")
}

func (d *FDDemultiplexer) Remove(fd int) {}

func (d *FDDemultiplexer) Close() error {
	return nil
}
