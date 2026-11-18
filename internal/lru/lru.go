// Package lru is a bounded, concurrency-safe least-recently-used cache.
package lru

import (
	"container/list"
	"sync"
)

// Cache holds at most max entries; a Get or Put makes an entry the most
// recent, and a Put past the bound drops the least recent one.
type Cache[K comparable, V any] struct {
	mu    sync.Mutex
	max   int
	order *list.List // front = most recent
	items map[K]*list.Element
}

type entry[K comparable, V any] struct {
	key K
	val V
}

// New returns an empty cache bounded to max entries.
func New[K comparable, V any](max int) *Cache[K, V] {
	return &Cache[K, V]{max: max, order: list.New(), items: map[K]*list.Element{}}
}

// Get returns the value for key and marks it most recent.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	c.order.MoveToFront(e)
	return e.Value.(*entry[K, V]).val, true
}

// Put stores value under key as the most recent entry.
func (c *Cache[K, V]) Put(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		e.Value.(*entry[K, V]).val = value
		c.order.MoveToFront(e)
		return
	}
	c.items[key] = c.order.PushFront(&entry[K, V]{key, value})
	for c.order.Len() > c.max {
		last := c.order.Back()
		c.order.Remove(last)
		delete(c.items, last.Value.(*entry[K, V]).key)
	}
}

// Len is the number of entries held.
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}
