package inmemory

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type idempotencyKeyID struct {
	userID  uint
	keyHash string
}

type IdempotencyKeyRepository struct {
	mu      sync.RWMutex
	records map[idempotencyKeyID]*domain.IdempotencyKey
	nextID  uint64
}

func NewIdempotencyKeyRepository() *IdempotencyKeyRepository {
	return &IdempotencyKeyRepository{
		records: make(map[idempotencyKeyID]*domain.IdempotencyKey),
	}
}

func (r *IdempotencyKeyRepository) Find(
	_ context.Context,
	userID uint,
	keyHash string,
	now time.Time,
) (*domain.IdempotencyKey, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	record, ok := r.records[idempotencyKeyID{userID: userID, keyHash: keyHash}]
	if !ok || record.IsExpired(now) {
		return nil, nil
	}

	return cloneIdempotencyKey(record), nil
}

func (r *IdempotencyKeyRepository) Save(_ context.Context, record *domain.IdempotencyKey) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := idempotencyKeyID{userID: record.UserID, keyHash: record.KeyHash}

	if existing, ok := r.records[id]; ok && !existing.IsExpired(record.CreatedAt) {
		return false, nil
	}

	r.nextID++
	record.ID = r.nextID
	r.records[id] = cloneIdempotencyKey(record)

	return true, nil
}

func (r *IdempotencyKeyRepository) DeleteExpired(_ context.Context, now time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	deleted := 0

	for id, record := range r.records {
		if record.IsExpired(now) {
			delete(r.records, id)
			deleted++
		}
	}

	return deleted, nil
}

// cloneIdempotencyKey keeps callers from mutating the stored record through
// the shared headers map or body slice.
func cloneIdempotencyKey(record *domain.IdempotencyKey) *domain.IdempotencyKey {
	clone := *record
	clone.ResponseBody = slices.Clone(record.ResponseBody)
	clone.ResponseHeaders = make(map[string][]string, len(record.ResponseHeaders))

	for name, values := range record.ResponseHeaders {
		clone.ResponseHeaders[name] = slices.Clone(values)
	}

	return &clone
}
