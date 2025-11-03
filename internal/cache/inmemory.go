package cache

import (
	"context"
	"sync"
	"time"
)

type item struct {
	value      any
	expiration time.Time
}

func (itm *item) isExpired() bool {
	if itm.expiration.IsZero() {
		return false
	}

	return time.Now().After(itm.expiration)
}

type InMemory struct {
	mu    sync.RWMutex
	items map[string]*item
}

func NewInMemory() *InMemory {
	return &InMemory{
		items: make(map[string]*item),
	}
}

func (c *InMemory) Get(_ context.Context, key string) (any, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return nil, ErrNotFound
	}

	if item.isExpired() {
		return nil, ErrNotFound
	}

	return item.value, nil
}

func (c *InMemory) Set(_ context.Context, key string, value any, options ...Option) error {
	opts := ApplyOptions(options...)

	var expiration time.Time
	if opts.Expiration > 0 {
		expiration = time.Now().Add(opts.Expiration)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = &item{
		value:      value,
		expiration: expiration,
	}

	return nil
}

func (c *InMemory) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)

	return nil
}

func (c *InMemory) Clear(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*item)

	return nil
}

func (c *InMemory) StartCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			c.cleanup()
		}
	}()
}

func (c *InMemory) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, item := range c.items {
		if item.isExpired() {
			delete(c.items, key)
		}
	}
}
