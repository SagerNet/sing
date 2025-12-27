//go:build !linux && !darwin && !windows

package bufio

import (
	"context"

	E "github.com/sagernet/sing/common/exceptions"
)

type FDPoller struct{}

func NewFDPoller(ctx context.Context) (*FDPoller, error) {
	return nil, E.New("FDPoller not supported on this platform")
}

func (p *FDPoller) Add(handler FDHandler, fd int) error {
	return E.New("FDPoller not supported on this platform")
}

func (p *FDPoller) Remove(fd int) {}

func (p *FDPoller) Close() error {
	return nil
}
