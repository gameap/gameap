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
