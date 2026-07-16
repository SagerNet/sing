package freelru

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testHash(value int) uint32 {
	return uint32(value) * 0x9E3779B1
}

func TestLRUGrowsToMaximumCapacity(t *testing.T) {
	lru, err := newLRU[int, int](4096, testHash)
	require.NoError(t, err)
	require.Equal(t, uint32(defaultInitialCapacity), lru.cap)
	require.Equal(t, uint32(4096), lru.maxCap)

	var evicted []int
	lru.SetOnEvict(func(key int, _ int) {
		evicted = append(evicted, key)
	})
	for key := 0; key < 4096; key++ {
		require.False(t, lru.Add(key, key))
	}
	require.Equal(t, uint32(4096), lru.cap)
	require.Equal(t, 4096, lru.Len())
	require.Empty(t, evicted)
	for key := 0; key < 4096; key++ {
		value, loaded := lru.Peek(key)
		require.True(t, loaded)
		require.Equal(t, key, value)
	}

	require.True(t, lru.Add(4096, 4096))
	require.Equal(t, []int{0}, evicted)
	_, loaded := lru.Peek(0)
	require.False(t, loaded)
}

func TestLRUGrowthRehashesCollisions(t *testing.T) {
	lru, err := newLRU[int, int](2048, func(int) uint32 { return 1 })
	require.NoError(t, err)
	for key := 0; key < 1536; key++ {
		lru.Add(key, key)
	}
	require.Equal(t, uint32(2048), lru.cap)
	for key := 0; key < 1536; key += 3 {
		require.True(t, lru.Remove(key))
	}
	for key := 0; key < 1536; key++ {
		value, loaded := lru.Peek(key)
		if key%3 == 0 {
			require.False(t, loaded)
		} else {
			require.True(t, loaded)
			require.Equal(t, key, value)
		}
	}
}

func TestLRUPurgeShrinksToInitialCapacity(t *testing.T) {
	lru, err := newLRUWithSize[int, int](4096, 8192, testHash)
	require.NoError(t, err)
	require.Equal(t, uint32(1024), lru.cap)
	require.Equal(t, uint32(2048), lru.size)
	for key := 0; key < 2048; key++ {
		lru.Add(key, key)
	}
	require.Equal(t, uint32(2048), lru.cap)
	require.Equal(t, uint32(4096), lru.size)

	lru.Purge()
	require.Zero(t, lru.Len())
	require.Equal(t, lru.minCap, lru.cap)
	require.Equal(t, lru.minSize, lru.size)
}

func TestLRUExpirationShrinksCapacity(t *testing.T) {
	lru, err := newLRU[int, int](4096, testHash)
	require.NoError(t, err)
	for key := 0; key < 2048; key++ {
		lru.AddWithLifetime(key, key, time.Millisecond)
	}
	require.Equal(t, uint32(2048), lru.cap)

	lru.purgeExpiredAt(now() + time.Second.Milliseconds())
	require.Zero(t, lru.Len())
	require.Equal(t, lru.minCap, lru.cap)
}

func TestShardedLRUGrowsIndividualShard(t *testing.T) {
	hash := func(key int) uint32 {
		return testHash(key) &^ (3 << 16)
	}
	lru, err := newShardedWithSize[int, int](4, 8192, 8192, hash)
	require.NoError(t, err)
	for shard := range lru.lrus {
		require.Equal(t, uint32(256), lru.lrus[shard].cap)
		require.Equal(t, uint32(2048), lru.lrus[shard].maxCap)
	}
	for key := 0; key < 512; key++ {
		lru.Add(key, key)
	}
	require.Equal(t, uint32(512), lru.lrus[0].cap)
	for shard := 1; shard < 4; shard++ {
		require.Equal(t, uint32(256), lru.lrus[shard].cap)
	}
}

func TestShardedGetAndRefreshOrAddSweepsExpiredShards(t *testing.T) {
	hash := func(key int) uint32 {
		if key >= 1000 {
			return uint32(key - 1000)
		}
		return uint32(key) << 16
	}
	lru, err := newShardedWithSize[int, int](4, 64, 64, hash)
	require.NoError(t, err)
	for key := 0; key < 4; key++ {
		lru.AddWithLifetime(key, key, 10*time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)

	for key := 1000; key < 1003; key++ {
		value, updated, ok := lru.GetAndRefreshOrAdd(key, func() (int, bool) {
			return key, true
		})
		require.Equal(t, key, value)
		require.False(t, updated)
		require.True(t, ok)
	}
	require.Equal(t, 3, lru.Len())
}
