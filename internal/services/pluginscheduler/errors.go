package pluginscheduler

import "github.com/pkg/errors"

var (
	ErrInvalidTask      = errors.New("invalid scheduled task")
	ErrTaskLimitReached = errors.New("scheduled task limit reached")
)
