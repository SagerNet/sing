package freelru_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/sagernet/sing/common"
	F "github.com/sagernet/sing/common/format"
	"github.com/sagernet/sing/contrab/freelru"
	"github.com/sagernet/sing/contrab/maphash"

	"github.com/stretchr/testify/require"
)

func TestUpdateLifetimeOnGet(t *testing.T) {
	t.Parallel()
	lru, err := freelru.New[string, string](1024, maphash.NewHasher[string]().Hash32, false)
	require.NoError(t, err)
	lru.AddWithLifetime("hello", "world", 2*time.Second)
	time.Sleep(time.Second)
	_, ok := lru.GetAndRefresh("hello")
	require.True(t, ok)
	time.Sleep(time.Second + time.Millisecond*100)
	_, ok = lru.Get("hello")
	require.True(t, ok)
}

func TestUpdateLifetimeOnGet1(t *testing.T) {
	t.Parallel()
	lru, err := freelru.New[string, string](1024, maphash.NewHasher[string]().Hash32, false)
	require.NoError(t, err)
	lru.AddWithLifetime("hello", "world", 2*time.Second)
	time.Sleep(time.Second)
	lru.Peek("hello")
	time.Sleep(time.Second + time.Millisecond*100)
	_, ok := lru.Get("hello")
	require.False(t, ok)
}

func TestUpdateLifetime(t *testing.T) {
	t.Parallel()
	lru, err := freelru.New[string, string](1024, maphash.NewHasher[string]().Hash32, false)
	require.NoError(t, err)
	lru.Add("hello", "world")
	require.True(t, lru.UpdateLifetime("hello", "world", 2*time.Second))
	time.Sleep(time.Second)
	_, ok := lru.Get("hello")
	require.True(t, ok)
	time.Sleep(time.Second + time.Millisecond*100)
	_, ok = lru.Get("hello")
	require.False(t, ok)
}

func TestUpdateLifetime1(t *testing.T) {
	t.Parallel()
	lru, err := freelru.New[string, string](1024, maphash.NewHasher[string]().Hash32, false)
	require.NoError(t, err)
	lru.Add("hello", "world")
	require.False(t, lru.UpdateLifetime("hello", "not world", 2*time.Second))
	time.Sleep(2*time.Second + time.Millisecond*100)
	_, ok := lru.Get("hello")
	require.True(t, ok)
}

func TestUpdateLifetime2(t *testing.T) {
	t.Parallel()
	lru, err := freelru.New[string, string](1024, maphash.NewHasher[string]().Hash32, false)
	require.NoError(t, err)
	lru.AddWithLifetime("hello", "world", 2*time.Second)
	time.Sleep(time.Second)
	require.True(t, lru.UpdateLifetime("hello", "world", 2*time.Second))
	time.Sleep(time.Second + time.Millisecond*100)
	_, ok := lru.Get("hello")
	require.True(t, ok)
	time.Sleep(time.Second + time.Millisecond*100)
	_, ok = lru.Get("hello")
	require.False(t, ok)
}

func TestUpdateLifetimePersistsAcrossRefresh(t *testing.T) {
	t.Parallel()
	lru, err := freelru.New[string, string](1024, maphash.NewHasher[string]().Hash32, false)
	require.NoError(t, err)
	lru.AddWithLifetime("hello", "world", 50*time.Millisecond)
	require.True(t, lru.UpdateLifetime("hello", "world", 500*time.Millisecond))
	value, updated, ok := lru.GetAndRefreshOrAdd("hello", func() (string, bool) {
		t.Fatal("constructor should not be called for an existing key")
		return "", false
	})
	require.True(t, ok)
	require.True(t, updated)
	require.Equal(t, "world", value)
	_, lifetime, ok := lru.PeekWithLifetime("hello")
	require.True(t, ok)
	require.Greater(t, time.Until(lifetime), 250*time.Millisecond)
}

func TestPurgeExpired(t *testing.T) {
	t.Parallel()
	lru, err := freelru.New[string, *string](1024, maphash.NewHasher[string]().Hash32, false)
	require.NoError(t, err)
	lru.SetLifetime(time.Second)
	lru.SetOnEvict(func(s string, s2 *string) {
		if s2 == nil {
			t.Fail()
		}
	})
	for i := 0; i < 100; i++ {
		lru.AddWithLifetime("hello_"+F.ToString(i), common.Ptr("world_"+F.ToString(i)), time.Duration(rand.Intn(3000))*time.Millisecond)
	}
	for i := 0; i < 5; i++ {
		time.Sleep(time.Second)
		lru.GetAndRefreshOrAdd("hellox"+F.ToString(i), func() (*string, bool) {
			return common.Ptr("worldx"), true
		})
	}
}

func TestGetAndRefreshOrAddPurgesBeforeEviction(t *testing.T) {
	lru, err := freelru.New[string, string](2, maphash.NewHasher[string]().Hash32, false)
	require.NoError(t, err)

	var evicted []string
	lru.SetOnEvict(func(key string, _ string) {
		evicted = append(evicted, key)
	})
	lru.AddWithLifetime("live", "live", time.Minute)
	lru.AddWithLifetime("expired", "expired", 10*time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	value, updated, ok := lru.GetAndRefreshOrAdd("new", func() (string, bool) {
		return "new", true
	})
	require.Equal(t, "new", value)
	require.False(t, updated)
	require.True(t, ok)
	require.Equal(t, 2, lru.Len())
	require.ElementsMatch(t, []string{"live", "new"}, lru.Keys())
	require.Equal(t, []string{"expired"}, evicted)
}

func TestGetAndRefreshOrAddKeepsRefreshedExpiredEntry(t *testing.T) {
	lru, err := freelru.New[string, string](2, maphash.NewHasher[string]().Hash32, false)
	require.NoError(t, err)
	lru.SetLifetime(20 * time.Millisecond)
	lru.Add("revived", "value")
	time.Sleep(40 * time.Millisecond)

	value, updated, ok := lru.GetAndRefreshOrAdd("revived", func() (string, bool) {
		t.Fatal("constructor should not be called when an expired entry is refreshed")
		return "", false
	})
	require.Equal(t, "value", value)
	require.True(t, updated)
	require.True(t, ok)
	require.True(t, lru.UpdateLifetime("revived", "value", time.Minute))

	_, updated, ok = lru.GetAndRefreshOrAdd("new", func() (string, bool) {
		return "new", true
	})
	require.False(t, updated)
	require.True(t, ok)
	require.ElementsMatch(t, []string{"revived", "new"}, lru.Keys())
}

func TestPurgeExpiredAfterDenseCompaction(t *testing.T) {
	lru, err := freelru.New[int, int](64, maphash.NewHasher[int]().Hash32, false)
	require.NoError(t, err)
	lru.AddWithLifetime(0, 0, time.Minute)
	for key := 1; key <= 32; key++ {
		lru.AddWithLifetime(key, key, 10*time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)

	lru.PurgeExpired()
	require.Equal(t, 1, lru.Len())
	require.Equal(t, []int{0}, lru.Keys())
}
