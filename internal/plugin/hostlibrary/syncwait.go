package hostlibrary

import (
	"context"
	"time"
)

// syncWaitGuestDeadlineGrace keeps a blocking host call strictly inside the
// guest call deadline: hitting the deadline itself would close the wasm module
// (WithCloseOnContextDone), while an expired wait merely answers
// completed=false and leaves the operation running.
const syncWaitGuestDeadlineGrace = 2 * time.Second

// defaultSyncWaitBudget applies when the request names no timeout and the
// context carries no deadline (the wrapper always sets one in production);
// without it such a call would answer completed=false immediately.
const defaultSyncWaitBudget = 30 * time.Second

// syncWaitBudget is how long a blocking host call may wait for a background
// operation. A non-positive result means the guest deadline is imminent:
// answering now beats being killed mid-wait.
func syncWaitBudget(ctx context.Context, requested time.Duration) time.Duration {
	budget := max(requested, 0)

	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline) - syncWaitGuestDeadlineGrace
		if budget <= 0 || remaining < budget {
			budget = remaining
		}

		return budget
	}

	if budget <= 0 {
		return defaultSyncWaitBudget
	}

	return budget
}
