package dlq_test

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/dlq"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPublisher struct {
	publishFunc func(ctx context.Context, channel string, msg *pubsub.Message) error
	callCount   int
}

func (m *mockPublisher) Publish(ctx context.Context, channel string, msg *pubsub.Message) error {
	m.callCount++
	if m.publishFunc != nil {
		return m.publishFunc(ctx, channel, msg)
	}

	return nil
}

func TestHandler_RecordFailure(t *testing.T) {
	t.Parallel()

	store := dlq.NewMemoryStore(100)
	publisher := &mockPublisher{}
	handler := dlq.NewHandler(store, publisher)

	ctx := context.Background()
	msg := &pubsub.Message{
		ID:      "msg-1",
		Channel: "test:channel",
		Type:    "test.type",
	}
	err := handler.RecordFailure(ctx, msg, "test:channel", errors.New("test error"), 3)
	require.NoError(t, err)

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	list, err := store.List(ctx, 10, 0)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "test:channel", list[0].Channel)
	assert.Equal(t, "test error", list[0].Error)
	assert.Equal(t, 3, list[0].AttemptCount)
}

func TestHandler_Reprocess_success(t *testing.T) {
	t.Parallel()

	store := dlq.NewMemoryStore(100)
	publisher := &mockPublisher{}
	handler := dlq.NewHandler(store, publisher)

	ctx := context.Background()
	msg := &pubsub.Message{
		ID:      "msg-1",
		Channel: "test:channel",
		Type:    "test.type",
	}
	err := handler.RecordFailure(ctx, msg, "test:channel", errors.New("test error"), 3)
	require.NoError(t, err)

	list, err := store.List(ctx, 10, 0)
	require.NoError(t, err)
	require.Len(t, list, 1)

	err = handler.Reprocess(ctx, list[0].ID)
	require.NoError(t, err)

	assert.Equal(t, 1, publisher.callCount)

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestHandler_Reprocess_not_found(t *testing.T) {
	t.Parallel()

	store := dlq.NewMemoryStore(100)
	publisher := &mockPublisher{}
	handler := dlq.NewHandler(store, publisher)

	ctx := context.Background()
	err := handler.Reprocess(ctx, "nonexistent-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message not found")
}

func TestHandler_ReprocessAll(t *testing.T) {
	t.Parallel()

	store := dlq.NewMemoryStore(100)
	publisher := &mockPublisher{}
	handler := dlq.NewHandler(store, publisher)

	ctx := context.Background()
	for i := range 3 {
		msg := &pubsub.Message{
			ID:      "msg-" + string(rune('a'+i)),
			Channel: "test:channel",
			Type:    "test.type",
		}
		err := handler.RecordFailure(ctx, msg, "test:channel", errors.New("test error"), 3)
		require.NoError(t, err)
	}

	processed, failed, err := handler.ReprocessAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, processed)
	assert.Equal(t, 0, failed)
	assert.Equal(t, 3, publisher.callCount)
}

func TestHandler_ReprocessAll_with_some_unsuccessful(t *testing.T) {
	t.Parallel()

	store := dlq.NewMemoryStore(100)
	callCount := 0
	publisher := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, msg *pubsub.Message) error {
			callCount++
			if msg.ID == "msg-b" {
				return errors.New("publish error")
			}

			return nil
		},
	}
	handler := dlq.NewHandler(store, publisher)

	ctx := context.Background()
	for i := range 3 {
		msg := &pubsub.Message{
			ID:      "msg-" + string(rune('a'+i)),
			Channel: "test:channel",
			Type:    "test.type",
		}
		err := handler.RecordFailure(ctx, msg, "test:channel", errors.New("test error"), 3)
		require.NoError(t, err)
	}

	processed, failed, err := handler.ReprocessAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, processed)
	assert.Equal(t, 1, failed)

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	list, err := store.List(ctx, 10, 0)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "msg-b", list[0].OriginalMsg.ID)
}

func TestHandler_Store(t *testing.T) {
	t.Parallel()

	store := dlq.NewMemoryStore(100)
	publisher := &mockPublisher{}
	handler := dlq.NewHandler(store, publisher)

	assert.NotNil(t, handler.Store())

	ctx := context.Background()
	failedMsg := &dlq.FailedMessage{
		ID:        "test-id",
		Channel:   "test",
		Error:     "error",
		FailedAt:  time.Now(),
		Processed: false,
	}
	err := handler.Store().Push(ctx, failedMsg)
	require.NoError(t, err)

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
