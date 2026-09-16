package plugin

import (
	"sync"
	"time"
)

// throttle admits the first occurrence of a key at once and then one per
// interval. It keeps at most maxKeys keys: when full it drops the keys whose
// interval has passed, and refuses new keys while it stays full, so what it
// guards (an audit stream) stays bounded whatever the request mix.
type throttle[K comparable] struct {
	mu       sync.Mutex
	interval time.Duration
	maxKeys  int
	last     map[K]time.Time
	now      func() time.Time
}

func newThrottle[K comparable](interval time.Duration, maxKeys int) *throttle[K] {
	return &throttle[K]{
		interval: interval,
		maxKeys:  maxKeys,
		last:     make(map[K]time.Time),
		now:      time.Now,
	}
}

func (t *throttle[K]) admit(key K) bool {
	if t == nil {
		return true
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()

	if last, seen := t.last[key]; seen {
		if now.Sub(last) < t.interval {
			return false
		}

		t.last[key] = now

		return true
	}

	if len(t.last) >= t.maxKeys {
		t.sweep(now)
	}

	if len(t.last) >= t.maxKeys {
		return false
	}

	t.last[key] = now

	return true
}

func (t *throttle[K]) sweep(now time.Time) {
	for key, last := range t.last {
		if now.Sub(last) >= t.interval {
			delete(t.last, key)
		}
	}
}
