package plugin

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// defaultMinCallBudget is the least time a guest call must have left when it
// wins the call gate. A caller that queued for most of its deadline would
// otherwise enter the guest with a budget it cannot honour: the runtime
// closes the module on the deadline (WithCloseOnContextDone) and the plugin
// is disabled over what was really a queueing delay.
const defaultMinCallBudget = 2 * time.Second

type callBudgetKey struct{}

// callBudget is what a caller may say about the two phases of a guest call:
// how long to wait for the gate and how much of the deadline the guest
// itself needs.
type callBudget struct {
	// queueTimeout bounds the wait for the call gate on top of the context
	// deadline; 0 waits until the context is done.
	queueTimeout time.Duration
	// minCallBudget is the floor below which the guest is not invoked and
	// the caller gets ErrPluginBusy instead; 0 selects the default.
	minCallBudget time.Duration
}

// WithCallQueueTimeout bounds how long a guest call may wait for the plugin's
// call gate, independently of the context deadline that bounds the whole
// call. A caller that gives up here gets ErrPluginBusy; the guest was never
// invoked.
func WithCallQueueTimeout(ctx context.Context, timeout time.Duration) context.Context {
	budget := callBudgetFrom(ctx)
	budget.queueTimeout = timeout

	return context.WithValue(ctx, callBudgetKey{}, budget)
}

// WithCallMinBudget sets how much of the context deadline must remain once
// the call wins the gate; with less the call answers ErrPluginBusy without
// touching the guest. A caller whose guest work takes longer than the
// default raises it, so a queued call never starts what it cannot finish.
func WithCallMinBudget(ctx context.Context, budget time.Duration) context.Context {
	b := callBudgetFrom(ctx)
	b.minCallBudget = budget

	return context.WithValue(ctx, callBudgetKey{}, b)
}

func callBudgetFrom(ctx context.Context) callBudget {
	budget, _ := ctx.Value(callBudgetKey{}).(callBudget)

	return budget
}

func (b callBudget) minBudget() time.Duration {
	if b.minCallBudget > 0 {
		return b.minCallBudget
	}

	return defaultMinCallBudget
}

// check refuses a call whose deadline leaves less than the floor.
func (b callBudget) check(ctx context.Context) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil
	}

	remaining := time.Until(deadline)
	if remaining >= b.minBudget() {
		return nil
	}

	return errors.Wrapf(ErrPluginBusy, "%s of the call budget left, at least %s required",
		remaining.Round(time.Millisecond), b.minBudget())
}
