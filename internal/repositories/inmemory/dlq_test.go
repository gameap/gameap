package inmemory_test

import (
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/dlq"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupDLQRepo pushes one failed message per id, in the given order, so the
// oldest message is always the first id.
func setupDLQRepo(t *testing.T, maxSize int, ids ...string) repositories.DLQRepository {
	t.Helper()

	repo := inmemory.NewDLQRepository(maxSize)

	for _, id := range ids {
		require.NoError(t, repo.Push(t.Context(), newFailedMessage(id)))
	}

	return repo
}

func newFailedMessage(id string) *dlq.FailedMessage {
	return &dlq.FailedMessage{
		ID: id,
		OriginalMsg: &pubsub.Message{
			ID:      id,
			Channel: "servers:1",
			Type:    "server.updated",
		},
		Channel:      "servers:1",
		Error:        "subscriber refused the message",
		AttemptCount: 3,
		FailedAt:     time.Date(2024, time.May, 1, 12, 0, 0, 0, time.UTC),
	}
}

func failedMessageIDs(messages []dlq.FailedMessage) []string {
	ids := make([]string, 0, len(messages))
	for _, msg := range messages {
		ids = append(ids, msg.ID)
	}

	return ids
}

func TestDLQRepository_PushStoresMessagesInArrivalOrder(t *testing.T) {
	t.Parallel()

	// ARRANGE
	repo := setupDLQRepo(t, 10, "a", "b", "c")

	// ACT
	stored, listErr := repo.List(t.Context(), 10, 0)
	count, countErr := repo.Count(t.Context())

	// ASSERT
	require.NoError(t, listErr)
	require.NoError(t, countErr)
	require.Len(t, stored, 3)
	assert.Equal(t, []string{"a", "b", "c"}, failedMessageIDs(stored), "messages must keep their arrival order")
	assert.Equal(t, 3, count, "every unprocessed message must be counted")
	assert.Equal(t, "servers:1", stored[0].Channel, "the failed channel must be persisted")
	assert.Equal(t, "subscriber refused the message", stored[0].Error, "the failure reason must be persisted")
	assert.Equal(t, 3, stored[0].AttemptCount, "the attempt count must be persisted")
	require.NotNil(t, stored[0].OriginalMsg, "the original message must be persisted")
	assert.Equal(t, "server.updated", stored[0].OriginalMsg.Type, "the original message type must be persisted")
	assert.False(t, stored[0].Processed, "a freshly pushed message must not be marked processed")
	assert.Nil(t, stored[0].ProcessedAt, "a freshly pushed message must not carry a processing time")
}

func TestDLQRepository_List(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		limit   int
		offset  int
		wantIDs []string
	}{
		{
			name:    "limit_caps_the_returned_window",
			limit:   2,
			offset:  0,
			wantIDs: []string{"a", "b"},
		},
		{
			name:    "offset_skips_the_oldest_messages",
			limit:   2,
			offset:  1,
			wantIDs: []string{"b", "c"},
		},
		{
			name:    "limit_beyond_the_queue_returns_the_remainder",
			limit:   10,
			offset:  2,
			wantIDs: []string{"c"},
		},
		{
			name:    "offset_past_the_end_returns_nothing",
			limit:   10,
			offset:  3,
			wantIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupDLQRepo(t, 10, "a", "b", "c")

			// ACT
			messages, err := repo.List(t.Context(), tt.limit, tt.offset)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, messages, len(tt.wantIDs))
			assert.Equal(t, tt.wantIDs, failedMessageIDs(messages), "unexpected page of the dead letter queue")
		})
	}
}

func TestDLQRepository_PopTakesTheOldestUnprocessedMessage(t *testing.T) {
	t.Parallel()

	// ARRANGE
	repo := setupDLQRepo(t, 10, "a", "b", "c")

	// ACT
	popped, err := repo.Pop(t.Context())

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, popped)
	assert.Equal(t, "a", popped.ID, "the oldest message must be popped first")

	remaining, listErr := repo.List(t.Context(), 10, 0)
	require.NoError(t, listErr)
	require.Len(t, remaining, 2)
	assert.Equal(t, []string{"b", "c"}, failedMessageIDs(remaining), "a popped message must be removed from the queue")
}

func TestDLQRepository_PopSkipsProcessedMessages(t *testing.T) {
	t.Parallel()

	// ARRANGE
	repo := setupDLQRepo(t, 10, "a", "b")
	require.NoError(t, repo.MarkProcessed(t.Context(), "a"))

	// ACT
	popped, err := repo.Pop(t.Context())

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, popped)
	assert.Equal(t, "b", popped.ID, "an already processed message must not be popped again")

	remaining, listErr := repo.List(t.Context(), 10, 0)
	require.NoError(t, listErr)
	require.Len(t, remaining, 1)
	assert.Equal(t, "a", remaining[0].ID, "the processed message must stay in the queue")
}

func TestDLQRepository_PopWithoutUnprocessedMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ids       []string
		processed []string
		wantError string
	}{
		{
			name:      "empty_queue",
			wantError: "dlq: queue is empty",
		},
		{
			name:      "every_message_already_processed",
			ids:       []string{"a", "b"},
			processed: []string{"a", "b"},
			wantError: "dlq: queue is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupDLQRepo(t, 10, tt.ids...)
			for _, id := range tt.processed {
				require.NoError(t, repo.MarkProcessed(t.Context(), id))
			}

			// ACT
			popped, err := repo.Pop(t.Context())

			// ASSERT
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			assert.ErrorIs(t, err, dlq.ErrEmpty, "callers match the sentinel to detect an exhausted queue")
			assert.Nil(t, popped, "no message may be returned together with an error")
		})
	}
}

func TestDLQRepository_MarkProcessed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		id            string
		wantCount     int
		wantProcessed []string
		wantRemaining []string
	}{
		{
			name:          "known_id_is_marked_and_leaves_the_count",
			id:            "b",
			wantCount:     2,
			wantProcessed: []string{"b"},
			wantRemaining: []string{"a", "b", "c"},
		},
		{
			name:          "unknown_id_is_ignored",
			id:            "missing",
			wantCount:     3,
			wantProcessed: []string{},
			wantRemaining: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupDLQRepo(t, 10, "a", "b", "c")

			// ACT
			err := repo.MarkProcessed(t.Context(), tt.id)

			// ASSERT
			require.NoError(t, err)

			count, countErr := repo.Count(t.Context())
			require.NoError(t, countErr)
			assert.Equal(t, tt.wantCount, count, "Count must only report unprocessed messages")

			messages, listErr := repo.List(t.Context(), 10, 0)
			require.NoError(t, listErr)
			require.Len(t, messages, len(tt.wantRemaining))
			assert.Equal(t, tt.wantRemaining, failedMessageIDs(messages),
				"marking a message processed must not remove it")

			processed := make([]string, 0, len(messages))
			for _, msg := range messages {
				if msg.Processed {
					processed = append(processed, msg.ID)
					assert.NotNil(t, msg.ProcessedAt, "a processed message must record when it was processed")
				}
			}
			assert.Equal(t, tt.wantProcessed, processed, "unexpected set of processed messages")
		})
	}
}

func TestDLQRepository_Delete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		wantIDs []string
	}{
		{
			name:    "known_id_is_removed",
			id:      "b",
			wantIDs: []string{"a", "c"},
		},
		{
			name:    "unknown_id_leaves_the_queue_untouched",
			id:      "missing",
			wantIDs: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupDLQRepo(t, 10, "a", "b", "c")

			// ACT
			err := repo.Delete(t.Context(), tt.id)

			// ASSERT
			require.NoError(t, err)

			messages, listErr := repo.List(t.Context(), 10, 0)
			require.NoError(t, listErr)
			require.Len(t, messages, len(tt.wantIDs))
			assert.Equal(t, tt.wantIDs, failedMessageIDs(messages), "unexpected queue content after Delete")

			count, countErr := repo.Count(t.Context())
			require.NoError(t, countErr)
			assert.Equal(t, len(tt.wantIDs), count, "Count must follow Delete")
		})
	}
}

func TestDLQRepository_PurgeEmptiesTheQueue(t *testing.T) {
	t.Parallel()

	// ARRANGE
	repo := setupDLQRepo(t, 10, "a", "b", "c")

	// ACT
	err := repo.Purge(t.Context())

	// ASSERT
	require.NoError(t, err)

	count, countErr := repo.Count(t.Context())
	require.NoError(t, countErr)
	assert.Equal(t, 0, count, "Purge must drop every message")

	messages, listErr := repo.List(t.Context(), 10, 0)
	require.NoError(t, listErr)
	assert.Empty(t, messages, "Purge must leave nothing to list")
}

func TestDLQRepository_PushEvictsTheOldestMessageWhenFull(t *testing.T) {
	t.Parallel()

	// ARRANGE
	repo := setupDLQRepo(t, 2, "a", "b")

	// ACT
	err := repo.Push(t.Context(), newFailedMessage("c"))

	// ASSERT
	require.NoError(t, err, "a full queue must accept the new message instead of reporting an error")

	messages, listErr := repo.List(t.Context(), 10, 0)
	require.NoError(t, listErr)
	require.Len(t, messages, 2)
	assert.Equal(t, []string{"b", "c"}, failedMessageIDs(messages),
		"the oldest message must be dropped to make room for the newest one")
}

func TestDLQRepository_NonPositiveMaxSizeKeepsMessages(t *testing.T) {
	t.Parallel()

	// ARRANGE
	repo := setupDLQRepo(t, 0, "a", "b", "c")

	// ACT
	messages, err := repo.List(t.Context(), 10, 0)

	// ASSERT
	require.NoError(t, err)
	require.Len(t, messages, 3)
	assert.Equal(t, []string{"a", "b", "c"}, failedMessageIDs(messages),
		"a non-positive max size must fall back to a usable default instead of dropping everything")
}
