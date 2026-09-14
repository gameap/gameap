// Package ratelimit provides token-bucket rate limiting keyed by client (an
// address, a user id), for use in front of request handlers.
package ratelimit

import (
	"container/list"
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	// DefaultMaxKeys bounds the number of clients a limiter tracks; the
	// least recently seen client is dropped when the table is full.
	DefaultMaxKeys = 65536
	// DefaultIdleTTL is how long an unseen client keeps its bucket.
	DefaultIdleTTL = 10 * time.Minute
)

// Limit is a token bucket: RPS tokens per second with Burst capacity. RPS 0
// disables the limit.
type Limit struct {
	RPS   float64
	Burst int
}

// Enabled reports whether the limit applies.
func (l Limit) Enabled() bool {
	return l.RPS > 0
}

func (l Limit) burst() int {
	if l.Burst > 0 {
		return l.Burst
	}

	return max(1, int(math.Ceil(l.RPS)))
}

// KeyedLimiter keeps one token bucket per key in a bounded, least recently
// used table. Buckets are local to the process: behind a load balancer every
// instance enforces the limit on its own share of the traffic.
type KeyedLimiter struct {
	mu        sync.Mutex
	limit     Limit
	maxKeys   int
	idleTTL   time.Duration
	now       func() time.Time
	entries   map[string]*list.Element
	order     *list.List // most recently seen at the front
	lastSweep time.Time
}

type entry struct {
	key      string
	bucket   *rate.Limiter
	lastSeen time.Time
}

// Option configures a KeyedLimiter.
type Option func(*KeyedLimiter)

// WithMaxKeys bounds the number of tracked keys (DefaultMaxKeys).
func WithMaxKeys(n int) Option {
	return func(k *KeyedLimiter) {
		if n > 0 {
			k.maxKeys = n
		}
	}
}

// WithIdleTTL sets how long an unseen key keeps its bucket (DefaultIdleTTL).
func WithIdleTTL(ttl time.Duration) Option {
	return func(k *KeyedLimiter) {
		if ttl > 0 {
			k.idleTTL = ttl
		}
	}
}

// WithClock replaces the time source (tests).
func WithClock(now func() time.Time) Option {
	return func(k *KeyedLimiter) {
		if now != nil {
			k.now = now
		}
	}
}

// NewKeyed builds a limiter applying limit to every key.
func NewKeyed(limit Limit, opts ...Option) *KeyedLimiter {
	k := &KeyedLimiter{
		limit:   limit,
		maxKeys: DefaultMaxKeys,
		idleTTL: DefaultIdleTTL,
		now:     time.Now,
		entries: make(map[string]*list.Element),
		order:   list.New(),
	}

	for _, opt := range opts {
		opt(k)
	}

	k.lastSweep = k.now()

	return k
}

// Allow consumes one token from the key's bucket. When the bucket is empty
// it reports false and how long until the next token becomes available. A
// nil or disabled limiter allows everything.
func (k *KeyedLimiter) Allow(key string) (bool, time.Duration) {
	if k == nil || !k.limit.Enabled() {
		return true, 0
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	now := k.now()
	k.sweep(now)

	e := k.touch(key, now)

	reservation := e.bucket.ReserveN(now, 1)
	if !reservation.OK() {
		return false, k.idleTTL
	}

	delay := reservation.DelayFrom(now)
	if delay <= 0 {
		return true, 0
	}

	// The token is not consumed by a refused request: give it back so the
	// next allowed one is not charged for it.
	reservation.CancelAt(now)

	return false, delay
}

// Len reports the number of tracked keys.
func (k *KeyedLimiter) Len() int {
	if k == nil {
		return 0
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	return len(k.entries)
}

// touch returns the key's entry, creating it when needed, and marks it as
// the most recently seen. A full table drops its least recently seen key.
func (k *KeyedLimiter) touch(key string, now time.Time) *entry {
	if element, ok := k.entries[key]; ok {
		e, _ := element.Value.(*entry)
		e.lastSeen = now
		k.order.MoveToFront(element)

		return e
	}

	for len(k.entries) >= k.maxKeys {
		k.evict(k.order.Back())
	}

	e := &entry{
		key:      key,
		bucket:   rate.NewLimiter(rate.Limit(k.limit.RPS), k.limit.burst()),
		lastSeen: now,
	}
	k.entries[key] = k.order.PushFront(e)

	return e
}

// sweep drops keys unseen for longer than idleTTL, at most once per half
// TTL. The list is ordered by recency, so it stops at the first live entry.
func (k *KeyedLimiter) sweep(now time.Time) {
	if now.Sub(k.lastSweep) < k.idleTTL/2 {
		return
	}

	k.lastSweep = now

	for element := k.order.Back(); element != nil; element = k.order.Back() {
		e, _ := element.Value.(*entry)
		if now.Sub(e.lastSeen) <= k.idleTTL {
			return
		}

		k.evict(element)
	}
}

func (k *KeyedLimiter) evict(element *list.Element) {
	if element == nil {
		return
	}

	e, _ := element.Value.(*entry)
	delete(k.entries, e.key)
	k.order.Remove(element)
}
