package testing

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/dlq"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// DLQRepositorySuite exercises the shared dead-letter-queue contract across the
// SQL drivers. The push/pop/ordering/empty behaviour must be identical on
// SQLite, MySQL and PostgreSQL; the driver-specific wiring only supplies the
// repository.
type DLQRepositorySuite struct {
	suite.Suite

	repo repositories.DLQRepository
	fn   func(t *testing.T) repositories.DLQRepository
}

func NewDLQRepositorySuite(
	fn func(t *testing.T) repositories.DLQRepository,
) *DLQRepositorySuite {
	return &DLQRepositorySuite{
		fn: fn,
	}
}

func (s *DLQRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

// newFailedMessage returns a fully populated failed message. failedAt drives
// the Pop ordering; it is truncated to seconds by callers so that MySQL DATETIME
// (second precision) and PostgreSQL/SQLite all order identically.
func newFailedMessage(id string, failedAt time.Time) *dlq.FailedMessage {
	return &dlq.FailedMessage{
		ID:           id,
		Channel:      "servers.events",
		Error:        "handler returned error",
		AttemptCount: 3,
		FailedAt:     failedAt,
		OriginalMsg: &pubsub.Message{
			ID:      id + "-orig",
			Channel: "servers.events",
			Type:    "server.updated",
			Payload: []byte(`{"server_id":42}`),
		},
	}
}

func (s *DLQRepositorySuite) TestDLQRepositoryPushPop() {
	ctx := context.Background()

	s.T().Run("push_then_pop_returns_stored_message", func(t *testing.T) {
		// ARRANGE — start from a known-empty queue (SQLite shares one in-memory
		// DB across suite tests, so previous rows must be cleared).
		require.NoError(t, s.repo.Purge(ctx))
		failedAt := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-roundtrip", failedAt)))

		// ACT
		got, err := s.repo.Pop(ctx)

		// ASSERT — Pop must return exactly what Push stored.
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "dlq-roundtrip", got.ID, "message id must round-trip")
		assert.Equal(t, "servers.events", got.Channel)
		assert.Equal(t, "handler returned error", got.Error)
		assert.Equal(t, 3, got.AttemptCount, "attempt count must round-trip")
		assert.False(t, got.FailedAt.IsZero(), "failed_at must round-trip as a real timestamp")
		require.NotNil(t, got.OriginalMsg, "the wrapped original message must round-trip")
		assert.Equal(t, "server.updated", got.OriginalMsg.Type)
		assert.Equal(t, []byte(`{"server_id":42}`), got.OriginalMsg.Payload,
			"the original message payload must survive the JSON round-trip")
	})

	s.T().Run("pop_returns_oldest_message_first", func(t *testing.T) {
		// ARRANGE — two messages one second apart (distinct at MySQL DATETIME
		// second precision).
		require.NoError(t, s.repo.Purge(ctx))
		base := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-older", base)))
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-newer", base.Add(time.Second))))

		// ACT
		first, err := s.repo.Pop(ctx)
		require.NoError(t, err)
		second, err := s.repo.Pop(ctx)
		require.NoError(t, err)

		// ASSERT — oldest failed_at is delivered first.
		require.NotNil(t, first)
		require.NotNil(t, second)
		assert.Equal(t, "dlq-older", first.ID, "Pop must deliver the oldest failed message first")
		assert.Equal(t, "dlq-newer", second.ID)
	})

	s.T().Run("pop_on_empty_queue_returns_err_empty", func(t *testing.T) {
		// ARRANGE
		require.NoError(t, s.repo.Purge(ctx))

		// ACT
		_, err := s.repo.Pop(ctx)

		// ASSERT
		require.ErrorIs(t, err, dlq.ErrEmpty, "popping an empty queue must report ErrEmpty")
	})

	s.T().Run("full_drain_then_pop_returns_err_empty", func(t *testing.T) {
		// ARRANGE — three queued messages.
		require.NoError(t, s.repo.Purge(ctx))
		base := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
		for i := range 3 {
			id := "dlq-drain-" + string(rune('a'+i))
			require.NoError(t, s.repo.Push(ctx, newFailedMessage(id, base.Add(time.Duration(i)*time.Second))))
		}

		// ACT — drain every message.
		for i := range 3 {
			msg, err := s.repo.Pop(ctx)
			require.NoError(t, err, "drain iteration %d must succeed", i)
			require.NotNil(t, msg)
		}
		_, err := s.repo.Pop(ctx)

		// ASSERT — once fully drained the guarded delete leaves nothing behind.
		require.ErrorIs(t, err, dlq.ErrEmpty,
			"a fully drained queue must report ErrEmpty rather than re-delivering")
	})
}

func (s *DLQRepositorySuite) TestDLQRepositoryList() {
	ctx := context.Background()

	s.T().Run("list_returns_messages_newest_first", func(t *testing.T) {
		// ARRANGE
		require.NoError(t, s.repo.Purge(ctx))
		base := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-list-a", base)))
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-list-b", base.Add(time.Second))))
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-list-c", base.Add(2*time.Second))))

		// ACT
		messages, err := s.repo.List(ctx, 10, 0)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, messages, 3)
		assert.Equal(t, "dlq-list-c", messages[0].ID, "List must order by failed_at DESC")
		assert.Equal(t, "dlq-list-b", messages[1].ID)
		assert.Equal(t, "dlq-list-a", messages[2].ID)

		msg := messages[0]
		assert.Equal(t, "servers.events", msg.Channel)
		assert.Equal(t, "handler returned error", msg.Error)
		assert.Equal(t, 3, msg.AttemptCount, "attempt count must round-trip")
		assert.False(t, msg.Processed, "fresh message must be unprocessed")
		assert.Nil(t, msg.ProcessedAt, "fresh message must have no processed_at")
		require.NotNil(t, msg.OriginalMsg, "the wrapped original message must round-trip")
		assert.Equal(t, "server.updated", msg.OriginalMsg.Type)
		assert.Equal(t, []byte(`{"server_id":42}`), msg.OriginalMsg.Payload)
	})

	s.T().Run("list_applies_limit_and_offset", func(t *testing.T) {
		// ARRANGE
		require.NoError(t, s.repo.Purge(ctx))
		base := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-page-a", base)))
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-page-b", base.Add(time.Second))))
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-page-c", base.Add(2*time.Second))))

		// ACT
		messages, err := s.repo.List(ctx, 1, 1)

		// ASSERT — failed_at DESC order is c,b,a; offset 1 limit 1 yields b.
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "dlq-page-b", messages[0].ID, "limit/offset must page over the DESC ordering")
	})

	s.T().Run("list_on_empty_queue_returns_no_messages", func(t *testing.T) {
		// ARRANGE
		require.NoError(t, s.repo.Purge(ctx))

		// ACT
		messages, err := s.repo.List(ctx, 10, 0)

		// ASSERT
		require.NoError(t, err)
		assert.Empty(t, messages)
	})
}

func (s *DLQRepositorySuite) TestDLQRepositoryCount() {
	ctx := context.Background()

	s.T().Run("count_tracks_unprocessed_messages", func(t *testing.T) {
		// ARRANGE
		require.NoError(t, s.repo.Purge(ctx))
		base := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)

		count, err := s.repo.Count(ctx)
		require.NoError(t, err)
		require.Equal(t, 0, count, "empty queue must count zero")

		for i := range 3 {
			id := "dlq-count-" + string(rune('a'+i))
			require.NoError(t, s.repo.Push(ctx, newFailedMessage(id, base.Add(time.Duration(i)*time.Second))))
		}

		// ACT
		count, err = s.repo.Count(ctx)

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, 3, count, "all pushed messages must be counted")
	})

	s.T().Run("count_excludes_processed_messages", func(t *testing.T) {
		// ARRANGE
		require.NoError(t, s.repo.Purge(ctx))
		base := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-count-p1", base)))
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-count-p2", base.Add(time.Second))))
		require.NoError(t, s.repo.MarkProcessed(ctx, "dlq-count-p1"))

		// ACT
		count, err := s.repo.Count(ctx)

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, 1, count, "processed messages must not be counted")
	})
}

func (s *DLQRepositorySuite) TestDLQRepositoryMarkProcessed() {
	ctx := context.Background()

	s.T().Run("mark_processed_sets_flag_and_timestamp", func(t *testing.T) {
		// ARRANGE
		require.NoError(t, s.repo.Purge(ctx))
		base := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-mark", base)))

		// ACT
		err := s.repo.MarkProcessed(ctx, "dlq-mark")

		// ASSERT
		require.NoError(t, err)
		messages, err := s.repo.List(ctx, 10, 0)
		require.NoError(t, err)
		require.Len(t, messages, 1, "processed message must stay listed")
		assert.True(t, messages[0].Processed, "message must be marked processed")
		assert.NotNil(t, messages[0].ProcessedAt, "processed_at must be set")
	})

	s.T().Run("mark_processed_excludes_message_from_pop", func(t *testing.T) {
		// ARRANGE
		require.NoError(t, s.repo.Purge(ctx))
		base := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-mark-old", base)))
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-mark-new", base.Add(time.Second))))
		require.NoError(t, s.repo.MarkProcessed(ctx, "dlq-mark-old"))

		// ACT
		got, err := s.repo.Pop(ctx)

		// ASSERT — the processed oldest message must be skipped.
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "dlq-mark-new", got.ID, "Pop must skip processed messages")
	})

	s.T().Run("mark_processed_unknown_id_does_not_error", func(t *testing.T) {
		// ARRANGE
		require.NoError(t, s.repo.Purge(ctx))

		// ACT
		err := s.repo.MarkProcessed(ctx, "dlq-no-such-id")

		// ASSERT
		require.NoError(t, err, "marking a missing message is a no-op, not an error")
	})
}

func (s *DLQRepositorySuite) TestDLQRepositoryDelete() {
	ctx := context.Background()

	s.T().Run("delete_removes_only_the_target_message", func(t *testing.T) {
		// ARRANGE
		require.NoError(t, s.repo.Purge(ctx))
		base := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-del-a", base)))
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-del-b", base.Add(time.Second))))

		// ACT
		err := s.repo.Delete(ctx, "dlq-del-a")

		// ASSERT
		require.NoError(t, err)
		count, err := s.repo.Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "only the deleted message must disappear")

		messages, err := s.repo.List(ctx, 10, 0)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "dlq-del-b", messages[0].ID)
	})

	s.T().Run("delete_unknown_id_does_not_error", func(t *testing.T) {
		// ARRANGE
		require.NoError(t, s.repo.Purge(ctx))
		require.NoError(t, s.repo.Push(ctx, newFailedMessage("dlq-del-c",
			time.Now().Add(-time.Minute).UTC().Truncate(time.Second))))

		// ACT
		err := s.repo.Delete(ctx, "dlq-no-such-id")

		// ASSERT
		require.NoError(t, err, "deleting a missing message is a no-op, not an error")
		count, err := s.repo.Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "existing messages must survive a no-op delete")
	})
}
