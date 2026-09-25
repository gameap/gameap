package idempotency

import "github.com/pkg/errors"

var (
	ErrInvalidKey        = errors.New("invalid Idempotency-Key header")
	ErrBodyTooLarge      = errors.New("request body is too large for an idempotent request")
	ErrRequestInProgress = errors.New("a request with this idempotency key is still being processed")
	ErrKeyReused         = errors.New("idempotency key has already been used for a different request")
)
