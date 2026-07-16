package freelru

import "time"

// Cache is a thread-safe LRU cache that can use either a single lock or
// multiple shards.
type Cache[K comparable, V comparable] struct {
	synced  *SyncedLRU[K, V]
	sharded *ShardedLRU[K, V]
}

// New creates a thread-safe, growable LRU cache with the given maximum capacity.
// When sharded is false, the cache preserves exact LRU behavior with a single
// lock. When sharded is true, it reduces lock contention at the cost of exact
// global LRU behavior.
func New[K comparable, V comparable](capacity uint32, hash HashKeyCallback[K], sharded bool) (*Cache[K, V], error) {
	if sharded {
		cache, err := newSharded[K, V](capacity, hash)
		if err != nil {
			return nil, err
		}
		return &Cache[K, V]{sharded: cache}, nil
	}
	cache, err := newSynced[K, V](capacity, hash)
	if err != nil {
		return nil, err
	}
	return &Cache[K, V]{synced: cache}, nil
}

func (lru *Cache[K, V]) SetLifetime(lifetime time.Duration) {
	if lru.sharded != nil {
		lru.sharded.SetLifetime(lifetime)
	} else {
		lru.synced.SetLifetime(lifetime)
	}
}

func (lru *Cache[K, V]) SetOnEvict(onEvict OnEvictCallback[K, V]) {
	if lru.sharded != nil {
		lru.sharded.SetOnEvict(onEvict)
	} else {
		lru.synced.SetOnEvict(onEvict)
	}
}

func (lru *Cache[K, V]) SetHealthCheck(healthCheck HealthCheckCallback[K, V]) {
	if lru.sharded != nil {
		lru.sharded.SetHealthCheck(healthCheck)
	} else {
		lru.synced.SetHealthCheck(healthCheck)
	}
}

func (lru *Cache[K, V]) Len() int {
	if lru.sharded != nil {
		return lru.sharded.Len()
	}
	return lru.synced.Len()
}

func (lru *Cache[K, V]) AddWithLifetime(key K, value V, lifetime time.Duration) bool {
	if lru.sharded != nil {
		return lru.sharded.AddWithLifetime(key, value, lifetime)
	}
	return lru.synced.AddWithLifetime(key, value, lifetime)
}

func (lru *Cache[K, V]) Add(key K, value V) bool {
	if lru.sharded != nil {
		return lru.sharded.Add(key, value)
	}
	return lru.synced.Add(key, value)
}

func (lru *Cache[K, V]) Get(key K) (V, bool) {
	if lru.sharded != nil {
		return lru.sharded.Get(key)
	}
	return lru.synced.Get(key)
}

func (lru *Cache[K, V]) GetWithLifetime(key K) (V, time.Time, bool) {
	if lru.sharded != nil {
		return lru.sharded.GetWithLifetime(key)
	}
	return lru.synced.GetWithLifetime(key)
}

func (lru *Cache[K, V]) GetWithLifetimeNoExpire(key K) (V, time.Time, bool) {
	if lru.sharded != nil {
		return lru.sharded.GetWithLifetimeNoExpire(key)
	}
	return lru.synced.GetWithLifetimeNoExpire(key)
}

func (lru *Cache[K, V]) GetAndRefresh(key K) (V, bool) {
	if lru.sharded != nil {
		return lru.sharded.GetAndRefresh(key)
	}
	return lru.synced.GetAndRefresh(key)
}

func (lru *Cache[K, V]) GetAndRefreshOrAdd(key K, constructor func() (V, bool)) (V, bool, bool) {
	if lru.sharded != nil {
		return lru.sharded.GetAndRefreshOrAdd(key, constructor)
	}
	return lru.synced.GetAndRefreshOrAdd(key, constructor)
}

func (lru *Cache[K, V]) Peek(key K) (V, bool) {
	if lru.sharded != nil {
		return lru.sharded.Peek(key)
	}
	return lru.synced.Peek(key)
}

func (lru *Cache[K, V]) PeekWithLifetime(key K) (V, time.Time, bool) {
	if lru.sharded != nil {
		return lru.sharded.PeekWithLifetime(key)
	}
	return lru.synced.PeekWithLifetime(key)
}

func (lru *Cache[K, V]) UpdateLifetime(key K, value V, lifetime time.Duration) bool {
	if lru.sharded != nil {
		return lru.sharded.UpdateLifetime(key, value, lifetime)
	}
	return lru.synced.UpdateLifetime(key, value, lifetime)
}

func (lru *Cache[K, V]) Contains(key K) bool {
	if lru.sharded != nil {
		return lru.sharded.Contains(key)
	}
	return lru.synced.Contains(key)
}

func (lru *Cache[K, V]) Remove(key K) bool {
	if lru.sharded != nil {
		return lru.sharded.Remove(key)
	}
	return lru.synced.Remove(key)
}

func (lru *Cache[K, V]) RemoveOldest() (K, V, bool) {
	if lru.sharded != nil {
		return lru.sharded.RemoveOldest()
	}
	return lru.synced.RemoveOldest()
}

func (lru *Cache[K, V]) Keys() []K {
	if lru.sharded != nil {
		return lru.sharded.Keys()
	}
	return lru.synced.Keys()
}

func (lru *Cache[K, V]) Purge() {
	if lru.sharded != nil {
		lru.sharded.Purge()
	} else {
		lru.synced.Purge()
	}
}

func (lru *Cache[K, V]) PurgeExpired() {
	if lru.sharded != nil {
		lru.sharded.PurgeExpired()
	} else {
		lru.synced.PurgeExpired()
	}
}

func (lru *Cache[K, V]) Metrics() Metrics {
	if lru.sharded != nil {
		return lru.sharded.Metrics()
	}
	return lru.synced.Metrics()
}

func (lru *Cache[K, V]) ResetMetrics() Metrics {
	if lru.sharded != nil {
		return lru.sharded.ResetMetrics()
	}
	return lru.synced.ResetMetrics()
}

func (lru *Cache[K, V]) dump() {
	if lru.sharded != nil {
		lru.sharded.dump()
	} else {
		lru.synced.dump()
	}
}

func (lru *Cache[K, V]) PrintStats() {
	if lru.sharded != nil {
		lru.sharded.PrintStats()
	} else {
		lru.synced.PrintStats()
	}
}
