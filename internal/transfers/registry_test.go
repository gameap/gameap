package transfers

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_WaitForPart_complete_with_zero_parts(t *testing.T) {
	t.Parallel()
	state := &State{ch: make(chan struct{})}
	state.Complete()

	ctx := context.Background()
	available, err := state.WaitForPart(ctx, 0)
	require.NoError(t, err)
	assert.False(t, available)
}

func TestState_WaitForPart_available_part(t *testing.T) {
	t.Parallel()
	state := &State{ch: make(chan struct{})}
	state.AddPart()
	state.Complete()

	ctx := context.Background()
	available, err := state.WaitForPart(ctx, 0)
	require.NoError(t, err)
	assert.True(t, available)
}

func TestState_WaitForPart_error(t *testing.T) {
	t.Parallel()
	state := &State{ch: make(chan struct{})}
	state.SetError(errors.New("transfer error"))

	ctx := context.Background()
	available, err := state.WaitForPart(ctx, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transfer error")
	assert.False(t, available)
}

func TestState_WaitForPart_context_cancelled(t *testing.T) {
	t.Parallel()
	state := &State{ch: make(chan struct{})}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	available, err := state.WaitForPart(ctx, 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, available)
}

func TestState_Complete_idempotent(t *testing.T) {
	t.Parallel()
	state := &State{ch: make(chan struct{})}

	state.Complete()
	state.Complete()

	ctx := context.Background()
	available, err := state.WaitForPart(ctx, 0)
	require.NoError(t, err)
	assert.False(t, available)
}

func TestState_Complete_after_SetError(t *testing.T) {
	t.Parallel()
	state := &State{ch: make(chan struct{})}

	state.SetError(errors.New("some error"))
	state.Complete()

	ctx := context.Background()
	available, err := state.WaitForPart(ctx, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "some error")
	assert.False(t, available)
}

func TestState_Complete_unblocks_waiting_goroutine(t *testing.T) {
	t.Parallel()
	state := &State{ch: make(chan struct{})}

	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx := context.Background()
		available, err := state.WaitForPart(ctx, 0)
		assert.NoError(t, err)
		assert.False(t, available)
	}()

	time.Sleep(10 * time.Millisecond)
	state.Complete()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WaitForPart did not unblock after Complete")
	}
}

func TestState_AddPart_then_Complete(t *testing.T) {
	t.Parallel()
	state := &State{ch: make(chan struct{})}

	state.AddPart()
	state.AddPart()
	state.Complete()

	ctx := context.Background()

	available, err := state.WaitForPart(ctx, 0)
	require.NoError(t, err)
	assert.True(t, available)

	available, err = state.WaitForPart(ctx, 1)
	require.NoError(t, err)
	assert.True(t, available)

	available, err = state.WaitForPart(ctx, 2)
	require.NoError(t, err)
	assert.False(t, available)
}

func TestRegistry_Register_Get_Unregister(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	state := reg.Register("transfer-1")
	require.NotNil(t, state)

	got, ok := reg.Get("transfer-1")
	require.True(t, ok)
	assert.Equal(t, state, got)

	reg.Unregister("transfer-1")

	_, ok = reg.Get("transfer-1")
	assert.False(t, ok)
}
