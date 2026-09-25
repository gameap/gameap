// Package idempotency implements the Idempotency-Key header for mutating API
// routes: the outcome of the first request made with a key is stored and
// replayed to every retry carrying the same key. See README.md.
package idempotency

import (
	"context"
	"net/http"
	"time"

	"github.com/gameap/gameap/internal/domain"
)

// Store keeps the outcomes of requests made with an Idempotency-Key.
type Store interface {
	// Find returns the user's live record for the key hash, or nil when there
	// is none or it has expired at now.
	Find(ctx context.Context, userID uint, keyHash string, now time.Time) (*domain.IdempotencyKey, error)

	// Save stores the record unless a live one already holds the key, which
	// it reports as false.
	Save(ctx context.Context, record *domain.IdempotencyKey) (bool, error)
}

type ExpiredDeleter interface {
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
}

// ReplayAuthorizer rechecks permissions enforced inside a handler before its
// stored response is returned. It must not mutate state or consume the body.
// Route-level authentication and authorization still wrap the middleware.
// A refusal is an error with a 4xx HTTPStatus; any other error means the
// check could not run and is answered with 503.
type ReplayAuthorizer interface {
	AuthorizeIdempotencyReplay(r *http.Request) error
}

type responder interface {
	WriteError(ctx context.Context, rw http.ResponseWriter, err error)
}

// statusError is an error that carries the HTTP status it is answered with.
type statusError interface {
	error
	HTTPStatus() int
}
