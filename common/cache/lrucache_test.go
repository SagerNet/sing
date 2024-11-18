package cache_test

import (
	"testing"
	"time"

	"github.com/sagernet/sing/common/cache"

	"github.com/stretchr/testify/require"
)

func TestLRUCache(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		t.Parallel()
		c := cache.New[string, int]()

		c.Store("key1", 1)
		value, exists := c.Load("key1")
		require.True(t, exists)
		require.Equal(t, 1, value)

		value, exists = c.Load("missing")
		require.False(t, exists)
		require.Zero(t, value)

		c.Delete("key1")
		_, exists = c.Load("key1")
		require.False(t, exists)
	})

	t.Run("max size", func(t *testing.T) {
		t.Parallel()
		c := cache.New[string, int](cache.WithSize[string, int](2))

		c.Store("key1", 1)
		c.Store("key2", 2)
		c.Store("key3", 3)

		_, exists := c.Load("key1")
		require.False(t, exists)

		value, exists := c.Load("key2")
		require.True(t, exists)
		require.Equal(t, 2, value)
	})

	t.Run("expiration", func(t *testing.T) {
		t.Parallel()
		c := cache.New[string, int](cache.WithAge[string, int](1))

		c.Store("key1", 1)

		value, exists := c.Load("key1")
		require.True(t, exists)
		require.Equal(t, 1, value)

		time.Sleep(time.Second * 2)

		value, exists = c.Load("key1")
		require.False(t, exists)
		require.Zero(t, value)
	})

	t.Run("clear", func(t *testing.T) {
		t.Parallel()
		evicted := make(map[string]int)
		c := cache.New[string, int](
			cache.WithEvict[string, int](func(key string, value int) {
				evicted[key] = value
			}),
		)

		c.Store("key1", 1)
		c.Store("key2", 2)

		c.Clear()

		require.Equal(t, map[string]int{"key1": 1, "key2": 2}, evicted)
		_, exists := c.Load("key1")
		require.False(t, exists)
	})

	t.Run("load or store", func(t *testing.T) {
		t.Parallel()
		c := cache.New[string, int]()

		value, loaded := c.LoadOrStore("key1", func() int { return 1 })
		require.False(t, loaded)
		require.Equal(t, 1, value)

		value, loaded = c.LoadOrStore("key1", func() int { return 2 })
		require.True(t, loaded)
		require.Equal(t, 1, value)
	})

	t.Run("update age on get", func(t *testing.T) {
		t.Parallel()
		c := cache.New[string, int](
			cache.WithAge[string, int](5),
			cache.WithUpdateAgeOnGet[string, int](),
		)

		c.Store("key1", 1)

		time.Sleep(time.Second * 3)
		_, exists := c.Load("key1")
		require.True(t, exists)

		time.Sleep(time.Second * 3)
		_, exists = c.Load("key1")
		require.True(t, exists)
	})
}
