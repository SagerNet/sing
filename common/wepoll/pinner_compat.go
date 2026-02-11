//go:build !go1.21

package wepoll

type Pinner struct{}

func (p *Pinner) Pin(pointer any) {}

func (p *Pinner) Unpin() {}
