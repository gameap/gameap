package hostlibrary

import (
	"maps"
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimit is a token bucket: RPS tokens per second, Burst capacity. RPS 0
// disables the limit for its class.
type RateLimit struct {
	RPS   float64
	Burst int
}

// Enabled reports whether the limit applies.
func (l RateLimit) Enabled() bool {
	return l.RPS > 0
}

func (l RateLimit) burst() int {
	if l.Burst > 0 {
		return l.Burst
	}

	return max(1, int(math.Ceil(l.RPS)))
}

type bucketKey struct {
	pluginID uint64
	class    RateClass
}

// rateLimiter keeps one token bucket per plugin and class, created lazily.
// Buckets are per panel instance: every instance runs its own copy of the
// plugin, so the effective cluster-wide rate is the limit times the number
// of instances.
type rateLimiter struct {
	mu      sync.Mutex
	limits  map[RateClass]RateLimit
	buckets map[bucketKey]*rate.Limiter
	now     func() time.Time
}

func newRateLimiter(limits map[RateClass]RateLimit) *rateLimiter {
	copied := make(map[RateClass]RateLimit, len(limits))
	maps.Copy(copied, limits)

	return &rateLimiter{
		limits:  copied,
		buckets: make(map[bucketKey]*rate.Limiter),
		now:     time.Now,
	}
}

// allow consumes one token for the plugin's bucket of the class. It reports
// whether the call may proceed and the limit that applied (zero value when
// the class is unlimited).
func (r *rateLimiter) allow(pluginID uint64, class RateClass) (bool, RateLimit) {
	if r == nil || class == "" {
		return true, RateLimit{}
	}

	limit, ok := r.limits[class]
	if !ok || !limit.Enabled() {
		return true, RateLimit{}
	}

	key := bucketKey{pluginID: pluginID, class: class}

	r.mu.Lock()
	defer r.mu.Unlock()

	bucket, ok := r.buckets[key]
	if !ok {
		bucket = rate.NewLimiter(rate.Limit(limit.RPS), limit.burst())
		r.buckets[key] = bucket
	}

	return bucket.AllowN(r.now(), 1), limit
}

// forget drops the plugin's buckets (uninstall).
func (r *rateLimiter) forget(pluginID uint64) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for key := range r.buckets {
		if key.pluginID == pluginID {
			delete(r.buckets, key)
		}
	}
}

// auditThrottleInterval spaces out the audit events for refused calls of one
// plugin and function, so a plugin looping on a refused call does not flood
// the audit stream; the metric still counts every refusal.
const auditThrottleInterval = time.Minute

type throttleKey struct {
	pluginID uint64
	module   string
	export   string
	reason   string
}

// auditThrottler admits the first refusal of each kind immediately and then
// one per interval.
type auditThrottler struct {
	mu       sync.Mutex
	interval time.Duration
	last     map[throttleKey]time.Time
	now      func() time.Time
}

func newAuditThrottler(interval time.Duration) *auditThrottler {
	return &auditThrottler{
		interval: interval,
		last:     make(map[throttleKey]time.Time),
		now:      time.Now,
	}
}

func (t *auditThrottler) admit(key throttleKey) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	if last, seen := t.last[key]; seen && now.Sub(last) < t.interval {
		return false
	}

	t.last[key] = now

	return true
}

func (t *auditThrottler) forget(pluginID uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for key := range t.last {
		if key.pluginID == pluginID {
			delete(t.last, key)
		}
	}
}
