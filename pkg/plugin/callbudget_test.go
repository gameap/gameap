// Security tests for the guest call budget.
//
// OWASP API Security Top 10:2023:
//   - API4:2023 Unrestricted Resource Consumption — a call that spent its
//     deadline queueing for the plugin's call gate must not enter the guest:
//     the runtime would close the module on the deadline and the plugin would
//     be disabled over a queueing delay, which is what a flood of cheap
//     requests produces.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package plugin

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCallFunction_refuses_a_call_with_less_than_the_budget_floor covers
// OWASP API4:2023: the guest function is nil on purpose, so entering the
// guest would panic — the floor has to answer before that.
func TestCallFunction_refuses_a_call_with_less_than_the_budget_floor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		deadline time.Duration
		floor    time.Duration
	}{
		{name: "default_floor", deadline: defaultMinCallBudget / 2},
		{name: "caller_floor", deadline: 200 * time.Millisecond, floor: 500 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			wrapper := &pluginServiceWrapper{gate: make(chan struct{}, 1)}

			ctx, cancel := context.WithTimeout(context.Background(), tt.deadline)
			defer cancel()

			if tt.floor > 0 {
				ctx = WithCallMinBudget(ctx, tt.floor)
			}

			// ACT
			_, err := wrapper.callFunction(ctx, nil, &proto.GetInfoRequest{})

			// ASSERT
			require.ErrorIs(t, err, ErrPluginBusy)
			assert.Contains(t, err.Error(), "of the call budget left")

			select {
			case wrapper.gate <- struct{}{}:
				<-wrapper.gate
			default:
				t.Fatal("the gate must be released when the call is refused")
			}
		})
	}
}

// TestCallFunction_floor_is_not_applied_without_a_deadline covers OWASP
// API4:2023: a call without a deadline gets the default call timeout, so
// there is nothing to compare the floor against. The guest function is nil,
// so the call fails inside the guest step, past the floor check.
func TestCallFunction_floor_is_not_applied_without_a_deadline(t *testing.T) {
	t.Parallel()

	// ARRANGE
	wrapper := &pluginServiceWrapper{gate: make(chan struct{}, 1)}

	// ACT + ASSERT
	assert.Panics(t, func() {
		_, _ = wrapper.callFunction(context.Background(), nil, &proto.GetInfoRequest{})
	}, "with no deadline the floor does not apply and the (nil) guest is reached")
}

// TestCallFunction_queue_timeout_bounds_the_wait covers OWASP API4:2023: the
// wait for the gate ends at the queue timeout even when the context deadline
// is far away, so a flood cannot hold a caller for its whole deadline.
func TestCallFunction_queue_timeout_bounds_the_wait(t *testing.T) {
	t.Parallel()

	// ARRANGE
	wrapper := &pluginServiceWrapper{gate: make(chan struct{}, 1)}
	wrapper.gate <- struct{}{}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ctx = WithCallQueueTimeout(ctx, 30*time.Millisecond)

	// ACT
	start := time.Now()
	_, err := wrapper.callFunction(ctx, nil, &proto.GetInfoRequest{})

	// ASSERT
	require.ErrorIs(t, err, ErrPluginBusy)
	assert.Less(t, time.Since(start), 5*time.Second, "the wait must end at the queue timeout")
}

func TestCallBudgetFrom(t *testing.T) {
	t.Parallel()

	t.Run("defaults_without_options", func(t *testing.T) {
		t.Parallel()

		budget := callBudgetFrom(context.Background())

		assert.Equal(t, time.Duration(0), budget.queueTimeout)
		assert.Equal(t, defaultMinCallBudget, budget.minBudget())
	})

	t.Run("options_compose", func(t *testing.T) {
		t.Parallel()

		ctx := WithCallQueueTimeout(context.Background(), time.Second)
		ctx = WithCallMinBudget(ctx, 3*time.Second)

		budget := callBudgetFrom(ctx)

		assert.Equal(t, time.Second, budget.queueTimeout)
		assert.Equal(t, 3*time.Second, budget.minBudget())
	})

	t.Run("options_survive_without_cancel", func(t *testing.T) {
		t.Parallel()

		ctx := WithCallMinBudget(context.Background(), 3*time.Second)

		budget := callBudgetFrom(context.WithoutCancel(ctx))

		assert.Equal(t, 3*time.Second, budget.minBudget())
	})
}
