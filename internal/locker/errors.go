package locker

import "github.com/pkg/errors"

var (
	ErrLocked   = errors.New("lock already held")
	ErrLockLost = errors.New("lock has been lost or expired")
)
