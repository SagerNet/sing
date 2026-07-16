package freelru

import "time"

// ConcurrentLRU is a thread-safe LRU cache that can use either a single lock
// or multiple shards without exposing the implementations through an interface.
type ConcurrentLRU[K comparable, V comparable] struct {
	synced  *SyncedLRU[K, V]
	sharded *ShardedLRU[K, V]
}

var _ Cache[int, int] = (*ConcurrentLRU[int, int])(nil)

// NewConcurrent creates a thread-safe LRU cache with the given capacity.
// When sharded is false, the cache preserves exact LRU behavior with a single
// lock. When sharded is true, it reduces lock contention at the cost of exact
// global LRU behavior.
func NewConcurrent[K comparable, V comparable](capacity uint32, hash HashKeyCallback[K], sharded bool) (*ConcurrentLRU[K, V], error) {
	if sharded {
		cache, err := NewSharded[K, V](capacity, hash)
		if err != nil {
			return nil, err
		}
		return &ConcurrentLRU[K, V]{sharded: cache}, nil
	}
	cache, err := NewSynced[K, V](capacity, hash)
	if err != nil {
		return nil, err
	}
	return &ConcurrentLRU[K, V]{synced: cache}, nil
}

func (lru *ConcurrentLRU[K, V]) SetLifetime(lifetime time.Duration) {
	if lru.sharded != nil {
		lru.sharded.SetLifetime(lifetime)
	} else {
		lru.synced.SetLifetime(lifetime)
	}
}

func (lru *ConcurrentLRU[K, V]) SetOnEvict(onEvict OnEvictCallback[K, V]) {
	if lru.sharded != nil {
		lru.sharded.SetOnEvict(onEvict)
	} else {
		lru.synced.SetOnEvict(onEvict)
	}
}

func (lru *ConcurrentLRU[K, V]) SetHealthCheck(healthCheck HealthCheckCallback[K, V]) {
	if lru.sharded != nil {
		lru.sharded.SetHealthCheck(healthCheck)
	} else {
		lru.synced.SetHealthCheck(healthCheck)
	}
}

func (lru *ConcurrentLRU[K, V]) Len() int {
	if lru.sharded != nil {
		return lru.sharded.Len()
	}
	return lru.synced.Len()
}

func (lru *ConcurrentLRU[K, V]) AddWithLifetime(key K, value V, lifetime time.Duration) bool {
	if lru.sharded != nil {
		return lru.sharded.AddWithLifetime(key, value, lifetime)
	}
	return lru.synced.AddWithLifetime(key, value, lifetime)
}

func (lru *ConcurrentLRU[K, V]) Add(key K, value V) bool {
	if lru.sharded != nil {
		return lru.sharded.Add(key, value)
	}
	return lru.synced.Add(key, value)
}

func (lru *ConcurrentLRU[K, V]) Get(key K) (V, bool) {
	if lru.sharded != nil {
		return lru.sharded.Get(key)
	}
	return lru.synced.Get(key)
}

func (lru *ConcurrentLRU[K, V]) GetWithLifetime(key K) (V, time.Time, bool) {
	if lru.sharded != nil {
		return lru.sharded.GetWithLifetime(key)
	}
	return lru.synced.GetWithLifetime(key)
}

func (lru *ConcurrentLRU[K, V]) GetWithLifetimeNoExpire(key K) (V, time.Time, bool) {
	if lru.sharded != nil {
		return lru.sharded.GetWithLifetimeNoExpire(key)
	}
	return lru.synced.GetWithLifetimeNoExpire(key)
}

func (lru *ConcurrentLRU[K, V]) GetAndRefresh(key K) (V, bool) {
	if lru.sharded != nil {
		return lru.sharded.GetAndRefresh(key)
	}
	return lru.synced.GetAndRefresh(key)
}

func (lru *ConcurrentLRU[K, V]) GetAndRefreshOrAdd(key K, constructor func() (V, bool)) (V, bool, bool) {
	if lru.sharded != nil {
		return lru.sharded.GetAndRefreshOrAdd(key, constructor)
	}
	return lru.synced.GetAndRefreshOrAdd(key, constructor)
}

func (lru *ConcurrentLRU[K, V]) Peek(key K) (V, bool) {
	if lru.sharded != nil {
		return lru.sharded.Peek(key)
	}
	return lru.synced.Peek(key)
}

func (lru *ConcurrentLRU[K, V]) PeekWithLifetime(key K) (V, time.Time, bool) {
	if lru.sharded != nil {
		return lru.sharded.PeekWithLifetime(key)
	}
	return lru.synced.PeekWithLifetime(key)
}

func (lru *ConcurrentLRU[K, V]) UpdateLifetime(key K, value V, lifetime time.Duration) bool {
	if lru.sharded != nil {
		return lru.sharded.UpdateLifetime(key, value, lifetime)
	}
	return lru.synced.UpdateLifetime(key, value, lifetime)
}

func (lru *ConcurrentLRU[K, V]) Contains(key K) bool {
	if lru.sharded != nil {
		return lru.sharded.Contains(key)
	}
	return lru.synced.Contains(key)
}

func (lru *ConcurrentLRU[K, V]) Remove(key K) bool {
	if lru.sharded != nil {
		return lru.sharded.Remove(key)
	}
	return lru.synced.Remove(key)
}

func (lru *ConcurrentLRU[K, V]) RemoveOldest() (K, V, bool) {
	if lru.sharded != nil {
		return lru.sharded.RemoveOldest()
	}
	return lru.synced.RemoveOldest()
}

func (lru *ConcurrentLRU[K, V]) Keys() []K {
	if lru.sharded != nil {
		return lru.sharded.Keys()
	}
	return lru.synced.Keys()
}

func (lru *ConcurrentLRU[K, V]) Purge() {
	if lru.sharded != nil {
		lru.sharded.Purge()
	} else {
		lru.synced.Purge()
	}
}

func (lru *ConcurrentLRU[K, V]) PurgeExpired() {
	if lru.sharded != nil {
		lru.sharded.PurgeExpired()
	} else {
		lru.synced.PurgeExpired()
	}
}

func (lru *ConcurrentLRU[K, V]) Metrics() Metrics {
	if lru.sharded != nil {
		return lru.sharded.Metrics()
	}
	return lru.synced.Metrics()
}

func (lru *ConcurrentLRU[K, V]) ResetMetrics() Metrics {
	if lru.sharded != nil {
		return lru.sharded.ResetMetrics()
	}
	return lru.synced.ResetMetrics()
}

func (lru *ConcurrentLRU[K, V]) dump() {
	if lru.sharded != nil {
		lru.sharded.dump()
	} else {
		lru.synced.dump()
	}
}

func (lru *ConcurrentLRU[K, V]) PrintStats() {
	if lru.sharded != nil {
		lru.sharded.PrintStats()
	} else {
		lru.synced.PrintStats()
	}
}
