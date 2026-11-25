package cache

// Modified by https://github.com/die-net/lrucache

import (
	"sync"
	"time"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/x/list"
)

type Option[K comparable, V any] func(*LruCache[K, V])

type EvictCallback[K comparable, V any] func(key K, value V)

func WithEvict[K comparable, V any](cb EvictCallback[K, V]) Option[K, V] {
	return func(l *LruCache[K, V]) {
		l.onEvict = cb
	}
}

func WithUpdateAgeOnGet[K comparable, V any]() Option[K, V] {
	return func(l *LruCache[K, V]) {
		l.updateAgeOnGet = true
	}
}

func WithAge[K comparable, V any](maxAge int64) Option[K, V] {
	return func(l *LruCache[K, V]) {
		l.maxAge = maxAge
	}
}

func WithSize[K comparable, V any](maxSize int) Option[K, V] {
	return func(l *LruCache[K, V]) {
		l.maxSize = maxSize
	}
}

type LruCache[K comparable, V any] struct {
	maxAge         int64
	maxSize        int
	mu             sync.Mutex
	cache          map[K]*list.Element[*entry[K, V]]
	lru            list.List[*entry[K, V]] // Front is least-recent
	updateAgeOnGet bool
	onEvict        EvictCallback[K, V]
}

func New[K comparable, V any](options ...Option[K, V]) *LruCache[K, V] {
	lc := &LruCache[K, V]{
		cache: make(map[K]*list.Element[*entry[K, V]]),
	}

	for _, option := range options {
		option(lc)
	}

	return lc
}

func (c *LruCache[K, V]) Load(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	le := c.loadLocked(key)
	if le == nil {
		return common.DefaultValue[V](), false
	}
	return le.Value.value, true
}

func (c *LruCache[K, V]) LoadOrStore(key K, constructor func() V) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if le := c.loadLocked(key); le != nil {
		return le.Value.value, true
	}
	value := constructor()
	c.cache[key] = c.lru.PushBack(&entry[K, V]{key: key, value: value, expires: time.Now().Unix() + c.maxAge})
	c.maybeDeleteOldest()
	return value, false
}

func (c *LruCache[K, V]) Exist(key K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, ok := c.cache[key]
	return ok
}

func (c *LruCache[K, V]) Store(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expires := time.Now().Unix() + c.maxAge
	if le, ok := c.cache[key]; ok {
		c.lru.MoveToBack(le)
		e := le.Value
		e.value = value
		e.expires = expires
	} else {
		e := &entry[K, V]{key: key, value: value, expires: expires}
		c.cache[key] = c.lru.PushBack(e)
	}

	c.maybeDeleteOldest()
}

func (c *LruCache[K, V]) CloneTo(n *LruCache[K, V]) {
	c.mu.Lock()
	defer c.mu.Unlock()

	n.mu.Lock()
	defer n.mu.Unlock()

	n.lru = list.List[*entry[K, V]]{}
	n.cache = make(map[K]*list.Element[*entry[K, V]])

	for e := c.lru.Front(); e != nil; e = e.Next() {
		elm := e.Value
		n.cache[elm.key] = n.lru.PushBack(elm)
	}
}

func (c *LruCache[K, V]) Range(block func(key K, value V)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for le := c.lru.Front(); le != nil; le = le.Next() {
		block(le.Value.key, le.Value.value)
	}
}

func (c *LruCache[K, V]) loadLocked(key K) *list.Element[*entry[K, V]] {
	le, ok := c.cache[key]
	if !ok {
		return nil
	}
	if c.maxAge > 0 && le.Value.expires <= time.Now().Unix() {
		c.deleteElement(le)
		c.maybeDeleteOldest()
		return nil
	}
	c.lru.MoveToBack(le)
	if c.maxAge > 0 && c.updateAgeOnGet {
		le.Value.expires = time.Now().Unix() + c.maxAge
	}
	return le
}

// Delete removes the value associated with a key.
func (c *LruCache[K, V]) Delete(key K) {
	c.mu.Lock()

	if le, ok := c.cache[key]; ok {
		c.deleteElement(le)
	}

	c.mu.Unlock()
}

func (c *LruCache[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for le := c.lru.Front(); le != nil; le = c.lru.Front() {
		c.deleteElement(le)
	}
}

func (c *LruCache[K, V]) maybeDeleteOldest() {
	if c.maxAge > 0 {
		now := time.Now().Unix()
		for le := c.lru.Front(); le != nil && le.Value.expires <= now; le = c.lru.Front() {
			c.deleteElement(le)
		}
	}

	if c.maxSize > 0 {
		if n := c.lru.Len(); n > c.maxSize {
			c.deleteElement(c.lru.Front())
		}
	}
}

func (c *LruCache[K, V]) deleteElement(le *list.Element[*entry[K, V]]) {
	c.lru.Remove(le)
	e := le.Value
	delete(c.cache, e.key)
	if c.onEvict != nil {
		c.onEvict(e.key, e.value)
	}
}

type entry[K comparable, V any] struct {
	key     K
	value   V
	expires int64
}
