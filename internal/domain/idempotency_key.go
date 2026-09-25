package domain

import "time"

// IdempotencyKey is the stored outcome of a mutating request sent with an
// Idempotency-Key header. A retry carrying the same key replays it instead of
// running the handler again. The client key and the request are kept only as
// keyed hashes: a request body may carry a password.
type IdempotencyKey struct {
	ID     uint64 `db:"id"`
	UserID uint   `db:"user_id"`

	// KeyHash identifies the client-supplied key within the user's scope.
	KeyHash string `db:"key_hash"`

	RequestMethod string `db:"request_method"`
	RequestPath   string `db:"request_path"`

	// RequestFingerprint tells a retry of the same request from a reuse of
	// the key for a different one.
	RequestFingerprint string `db:"request_fingerprint"`

	ResponseStatus  int                 `db:"response_status"`
	ResponseHeaders map[string][]string `db:"response_headers"`
	ResponseBody    []byte              `db:"response_body"`

	CreatedAt time.Time `db:"created_at"`
	ExpiresAt time.Time `db:"expires_at"`
}

// IsExpired reports whether the record no longer blocks its key at now.
func (k *IdempotencyKey) IsExpired(now time.Time) bool {
	return !now.Before(k.ExpiresAt)
}
