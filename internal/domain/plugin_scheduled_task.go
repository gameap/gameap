package domain

import "time"

type PluginScheduledTaskErrorPolicy string

const (
	PluginScheduledTaskErrorPolicyIgnore PluginScheduledTaskErrorPolicy = "ignore"
	PluginScheduledTaskErrorPolicyRetry  PluginScheduledTaskErrorPolicy = "retry"
)

func NewPluginScheduledTaskErrorPolicyFromString(s string) PluginScheduledTaskErrorPolicy {
	if s == string(PluginScheduledTaskErrorPolicyRetry) {
		return PluginScheduledTaskErrorPolicyRetry
	}

	return PluginScheduledTaskErrorPolicyIgnore
}

// PluginScheduledTask is a periodic task definition registered by a plugin
// via the gameap-scheduler host module. Run state (slots, running) is not
// persisted: instances coordinate through distributed locks (internal/locker).
type PluginScheduledTask struct {
	ID          uint64                         `db:"id"`
	PluginID    Uint64ID                       `db:"plugin_id"`
	Name        string                         `db:"name"`
	Interval    time.Duration                  `db:"interval_ms"`
	ErrorPolicy PluginScheduledTaskErrorPolicy `db:"error_policy"`
	MaxRetries  uint                           `db:"max_retries"`
	RetryDelay  time.Duration                  `db:"retry_delay_ms"`
	// MaxJitter is the upper bound of the random addition to each retry delay.
	MaxJitter time.Duration `db:"max_jitter_ms"`
	// Timeout bounds one handler call; zero means the scheduler default.
	Timeout   time.Duration `db:"timeout_ms"`
	CreatedAt *time.Time    `db:"created_at"`
	UpdatedAt *time.Time    `db:"updated_at"`
}
