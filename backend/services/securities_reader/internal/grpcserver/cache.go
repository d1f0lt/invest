package grpcserver

import (
	"sync"
	"time"
)

type ttlCache[V any] struct {
	mu    sync.Mutex
	ttl   time.Duration
	max   int
	items map[string]cacheEntry[V]
}

type cacheEntry[V any] struct {
	value   V
	expires time.Time
}

func newTTLCache[V any](ttl time.Duration, max int) *ttlCache[V] {
	return &ttlCache[V]{ttl: ttl, max: max, items: make(map[string]cacheEntry[V])}
}

func (c *ttlCache[V]) get(key string, now time.Time) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok || now.After(e.expires) {
		var zero V
		return zero, false
	}
	return e.value, true
}

func (c *ttlCache[V]) put(key string, value V, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.max {
		for k, e := range c.items {
			if now.After(e.expires) {
				delete(c.items, k)
			}
		}
		if len(c.items) >= c.max {
			c.items = make(map[string]cacheEntry[V])
		}
	}
	c.items[key] = cacheEntry[V]{value: value, expires: now.Add(c.ttl)}
}
