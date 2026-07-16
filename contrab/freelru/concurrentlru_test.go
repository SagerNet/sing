package freelru_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/sagernet/sing/contrab/freelru"
	"github.com/sagernet/sing/contrab/maphash"

	"github.com/stretchr/testify/require"
)

func TestConcurrentLRU(t *testing.T) {
	for _, sharded := range []bool{false, true} {
		t.Run(fmt.Sprintf("sharded=%v", sharded), func(t *testing.T) {
			cache, err := freelru.NewConcurrent[int, int](16, maphash.NewHasher[int]().Hash32, sharded)
			require.NoError(t, err)
			cache.SetLifetime(time.Minute)

			value, updated, ok := cache.GetAndRefreshOrAdd(1, func() (int, bool) {
				return 2, true
			})
			require.Equal(t, 2, value)
			require.False(t, updated)
			require.True(t, ok)

			value, updated, ok = cache.GetAndRefreshOrAdd(1, func() (int, bool) {
				t.Fatal("constructor called for existing key")
				return 0, false
			})
			require.Equal(t, 2, value)
			require.True(t, updated)
			require.True(t, ok)
			require.Equal(t, []int{1}, cache.Keys())
		})
	}
}

func TestConcurrentLRUConstructorDoesNotEscape(t *testing.T) {
	for _, sharded := range []bool{false, true} {
		t.Run(fmt.Sprintf("sharded=%v", sharded), func(t *testing.T) {
			cache, err := freelru.NewConcurrent[int, int](16, maphash.NewHasher[int]().Hash32, sharded)
			require.NoError(t, err)
			cache.Add(1, 2)
			state := [24]uint64{}
			allocations := testing.AllocsPerRun(1000, func() {
				value, updated, ok := cache.GetAndRefreshOrAdd(1, func() (int, bool) {
					return int(state[0]), true
				})
				if value != 2 || !updated || !ok {
					t.Fatal("unexpected cache result")
				}
			})
			require.Zero(t, allocations)
		})
	}
}

func BenchmarkConcurrentLRUGetAndRefreshOrAdd(b *testing.B) {
	hasher := maphash.NewHasher[int]()
	synced, err := freelru.NewSynced[int, int](1024, hasher.Hash32)
	require.NoError(b, err)
	b.Run("synced", func(b *testing.B) {
		benchmarkSyncedLRUGetAndRefreshOrAdd(b, synced)
	})
	concurrentSynced, err := freelru.NewConcurrent[int, int](1024, hasher.Hash32, false)
	require.NoError(b, err)
	b.Run("concurrent_synced", func(b *testing.B) {
		benchmarkConcurrentLRUGetAndRefreshOrAdd(b, concurrentSynced)
	})
	sharded, err := freelru.NewSharded[int, int](1024, hasher.Hash32)
	require.NoError(b, err)
	b.Run("sharded", func(b *testing.B) {
		benchmarkShardedLRUGetAndRefreshOrAdd(b, sharded)
	})
	concurrentSharded, err := freelru.NewConcurrent[int, int](1024, hasher.Hash32, true)
	require.NoError(b, err)
	b.Run("concurrent_sharded", func(b *testing.B) {
		benchmarkConcurrentLRUGetAndRefreshOrAdd(b, concurrentSharded)
	})
}

func benchmarkSyncedLRUGetAndRefreshOrAdd(b *testing.B, cache *freelru.SyncedLRU[int, int]) {
	cache.Add(1, 2)
	state := [24]uint64{}
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = cache.GetAndRefreshOrAdd(1, func() (int, bool) {
			return int(state[0]), true
		})
	}
}

func benchmarkShardedLRUGetAndRefreshOrAdd(b *testing.B, cache *freelru.ShardedLRU[int, int]) {
	cache.Add(1, 2)
	state := [24]uint64{}
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = cache.GetAndRefreshOrAdd(1, func() (int, bool) {
			return int(state[0]), true
		})
	}
}

func benchmarkConcurrentLRUGetAndRefreshOrAdd(b *testing.B, cache *freelru.ConcurrentLRU[int, int]) {
	cache.Add(1, 2)
	state := [24]uint64{}
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = cache.GetAndRefreshOrAdd(1, func() (int, bool) {
			return int(state[0]), true
		})
	}
}
