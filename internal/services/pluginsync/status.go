package pluginsync

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

// State is what the admin API reports for one plugin on this instance.
type State string

const (
	// StateInSync means the runtime matches the database row.
	StateInSync State = "in_sync"
	// StateRetrying means the last attempt failed (or hit contention) and
	// another one is scheduled.
	StateRetrying State = "retrying"
	// StateFailed means the last attempt failed and nothing is scheduled:
	// the row is in status "error" and only a change to it (or a repaired
	// file) triggers another attempt.
	StateFailed State = "failed"
)

// Status is a per-plugin snapshot of what this instance did. It is local by
// design: there is no cross-instance aggregation, so an operator reads it from
// whichever instance they are talking to.
type Status struct {
	PluginID    domain.Uint64ID
	State       State
	Failures    int
	LastError   string
	LastAttempt time.Time
	NextAttempt time.Time
}

// pluginState is what this instance recorded about one plugin between passes.
type pluginState struct {
	// fingerprint of the row the last attempt was made for; a different row
	// resets the backoff.
	fingerprint string
	// listenEvents is the grant as last observed for a present module, so a
	// change rebuilds the subscriptions even though nothing was reloaded.
	listenEvents bool
	failures     int
	lastErr      string
	lastAttempt  time.Time
	nextAttempt  time.Time
	scheduled    bool
}

func (st *pluginState) status(id domain.Uint64ID) Status {
	status := Status{
		PluginID:    id,
		State:       StateInSync,
		Failures:    st.failures,
		LastError:   st.lastErr,
		LastAttempt: st.lastAttempt,
		NextAttempt: st.nextAttempt,
	}

	switch {
	case st.lastErr == "":
	case st.scheduled:
		status.State = StateRetrying
	default:
		status.State = StateFailed
	}

	return status
}
