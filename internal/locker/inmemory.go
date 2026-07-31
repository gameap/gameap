package locker

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// InMemoryLocker provides a process-local lock. Suitable for single-instance
// deployments only. For multi-instance deployments, use RedisLocker or DBLocker.
type InMemoryLocker struct {
	mu    sync.Mutex
	locks map[string]*memEntry
}

type memEntry struct {
	expiresAt time.Time
	token     string
}

func NewInMemoryLocker() *InMemoryLocker {
	return &InMemoryLocker{
		locks: make(map[string]*memEntry),
	}
}

func (l *InMemoryLocker) Acquire(_ context.Context, key string, ttl time.Duration) (Lock, error) {
	if ttl <= 0 {
		return nil, errors.New("ttl must be positive")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// Slot-style keys are unique per occurrence and are never re-acquired,
	// so expired entries must be reclaimed or the map grows unboundedly.
	for k, entry := range l.locks {
		if !entry.expiresAt.After(now) {
			delete(l.locks, k)
		}
	}

	if entry, ok := l.locks[key]; ok && entry.expiresAt.After(now) {
		return nil, ErrLocked
	}

	token, err := newToken()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate lock token")
	}

	l.locks[key] = &memEntry{
		expiresAt: now.Add(ttl),
		token:     token,
	}

	return &memLock{
		owner: l,
		key:   key,
		token: token,
	}, nil
}

type memLock struct {
	mu       sync.Mutex
	owner    *InMemoryLocker
	key      string
	token    string
	released bool
}

func (l *memLock) Release(_ context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.released {
		return nil
	}

	l.owner.mu.Lock()
	defer l.owner.mu.Unlock()

	if entry, ok := l.owner.locks[l.key]; ok && entry.token == l.token {
		delete(l.owner.locks, l.key)
	}

	l.released = true

	return nil
}

func (l *memLock) Refresh(_ context.Context, ttl time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.released {
		return errors.New("lock already released")
	}

	if ttl <= 0 {
		return errors.New("ttl must be positive")
	}

	l.owner.mu.Lock()
	defer l.owner.mu.Unlock()

	now := time.Now()

	// An expired lock is lost even when nobody has stolen it yet, matching
	// the Redis backend where the key is physically gone after the TTL.
	entry, ok := l.owner.locks[l.key]
	if !ok || entry.token != l.token || !entry.expiresAt.After(now) {
		return ErrLockLost
	}

	entry.expiresAt = now.Add(ttl)

	return nil
}
