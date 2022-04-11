package replay

import (
	"sync"

	"github.com/v2fly/ss-bloomring"
)

func NewBloomRing() Filter {
	const (
		DefaultSFCapacity = 1e6
		DefaultSFFPR      = 1e-6
		DefaultSFSlot     = 10
	)
	return &bloomRingFilter{BloomRing: ss_bloomring.NewBloomRing(DefaultSFSlot, DefaultSFCapacity, DefaultSFFPR)}
}

type bloomRingFilter struct {
	sync.Mutex
	*ss_bloomring.BloomRing
}

func (f *bloomRingFilter) Check(sum []byte) bool {
	f.Lock()
	defer f.Unlock()
	if f.Test(sum) {
		return false
	}
	f.Add(sum)
	return true
}
