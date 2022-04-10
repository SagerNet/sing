package replay

import (
	"sync"
	"time"

	"github.com/seiflotfy/cuckoofilter"
)

func NewCuckoo(interval int64) Filter {
	filter := &cuckooFilter{}
	filter.interval = interval
	return filter
}

type cuckooFilter struct {
	lock     sync.Mutex
	poolA    *cuckoo.Filter
	poolB    *cuckoo.Filter
	poolSwap bool
	lastSwap int64
	interval int64
}

func (filter *cuckooFilter) Check(sum []byte) bool {
	const defaultCapacity = 100000

	filter.lock.Lock()
	defer filter.lock.Unlock()

	now := time.Now().Unix()
	if filter.lastSwap == 0 {
		filter.lastSwap = now
		filter.poolA = cuckoo.NewFilter(defaultCapacity)
		filter.poolB = cuckoo.NewFilter(defaultCapacity)
	}

	elapsed := now - filter.lastSwap
	if elapsed >= filter.interval {
		if filter.poolSwap {
			filter.poolA.Reset()
		} else {
			filter.poolB.Reset()
		}
		filter.poolSwap = !filter.poolSwap
		filter.lastSwap = now
	}

	return filter.poolA.InsertUnique(sum) && filter.poolB.InsertUnique(sum)
}
