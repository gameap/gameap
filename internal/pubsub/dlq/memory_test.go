package dlq_test

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/dlq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStore_Push_and_Pop(t *testing.T) {
	t.Parallel()

	store := dlq.NewMemoryStore(100)
	ctx := context.Background()

	msg := &dlq.FailedMessage{
		ID: "test-1",
		OriginalMsg: &pubsub.Message{
			ID:      "orig-1",
			Channel: "test",
			Type:    "test.type",
		},
		Channel:      "test",
		Error:        "test error",
		AttemptCount: 3,
		FailedAt:     time.Now(),
		Processed:    false,
	}

	err := store.Push(ctx, msg)
	require.NoError(t, err)

	popped, err := store.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, msg.ID, popped.ID)
	assert.Equal(t, msg.OriginalMsg.ID, popped.OriginalMsg.ID)
	assert.Equal(t, msg.Error, popped.Error)
}

func TestMemoryStore_Pop_returns_ErrEmpty(t *testing.T) {
	t.Parallel()

	store := dlq.NewMemoryStore(100)
	ctx := context.Background()

	_, err := store.Pop(ctx)
	assert.ErrorIs(t, err, dlq.ErrEmpty)
}

func TestMemoryStore_List(t *testing.T) {
	t.Parallel()

	store := dlq.NewMemoryStore(100)
	ctx := context.Background()

	for i := range 5 {
		msg := &dlq.FailedMessage{
			ID:           "test-" + string(rune('a'+i)),
			Channel:      "test",
			Error:        "error",
			AttemptCount: 1,
			FailedAt:     time.Now(),
		}
		err := store.Push(ctx, msg)
		require.NoError(t, err)
	}

	list, err := store.List(ctx, 3, 0)
	require.NoError(t, err)
	require.Len(t, list, 3)

	list, err = store.List(ctx, 10, 3)
	require.NoError(t, err)
	require.Len(t, list, 2)
}

func TestMemoryStore_Count(t *testing.T) {
	t.Parallel()

	store := dlq.NewMemoryStore(100)
	ctx := context.Background()

	for i := range 3 {
		msg := &dlq.FailedMessage{
			ID:        "test-" + string(rune('a'+i)),
			Channel:   "test",
			Error:     "error",
			FailedAt:  time.Now(),
			Processed: i == 1,
		}
		err := store.Push(ctx, msg)
		require.NoError(t, err)
	}

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestMemoryStore_MarkProcessed(t *testing.T) {
	t.Parallel()

	store := dlq.NewMemoryStore(100)
	ctx := context.Background()

	msg := &dlq.FailedMessage{
		ID:        "test-1",
		Channel:   "test",
		Error:     "error",
		FailedAt:  time.Now(),
		Processed: false,
	}
	err := store.Push(ctx, msg)
	require.NoError(t, err)

	err = store.MarkProcessed(ctx, "test-1")
	require.NoError(t, err)

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestMemoryStore_Delete(t *testing.T) {
	t.Parallel()

	store := dlq.NewMemoryStore(100)
	ctx := context.Background()

	msg := &dlq.FailedMessage{
		ID:       "test-1",
		Channel:  "test",
		Error:    "error",
		FailedAt: time.Now(),
	}
	err := store.Push(ctx, msg)
	require.NoError(t, err)

	err = store.Delete(ctx, "test-1")
	require.NoError(t, err)

	_, err = store.Pop(ctx)
	assert.ErrorIs(t, err, dlq.ErrEmpty)
}

func TestMemoryStore_Purge(t *testing.T) {
	t.Parallel()

	store := dlq.NewMemoryStore(100)
	ctx := context.Background()

	for i := range 5 {
		msg := &dlq.FailedMessage{
			ID:       "test-" + string(rune('a'+i)),
			Channel:  "test",
			Error:    "error",
			FailedAt: time.Now(),
		}
		err := store.Push(ctx, msg)
		require.NoError(t, err)
	}

	err := store.Purge(ctx)
	require.NoError(t, err)

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestMemoryStore_maxSize_eviction(t *testing.T) {
	t.Parallel()

	store := dlq.NewMemoryStore(3)
	ctx := context.Background()

	for i := range 5 {
		msg := &dlq.FailedMessage{
			ID:       "test-" + string(rune('a'+i)),
			Channel:  "test",
			Error:    "error",
			FailedAt: time.Now(),
		}
		err := store.Push(ctx, msg)
		require.NoError(t, err)
	}

	list, err := store.List(ctx, 10, 0)
	require.NoError(t, err)
	require.Len(t, list, 3)

	assert.Equal(t, "test-c", list[0].ID)
	assert.Equal(t, "test-d", list[1].ID)
	assert.Equal(t, "test-e", list[2].ID)
}
