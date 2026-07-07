package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub/dlq"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDLQRepositoryPop(t *testing.T) {
	db := SetupTestDB(t)
	repo := sqlite.NewDLQRepository(db)
	ctx := context.Background()

	_, err := repo.Pop(ctx)
	require.ErrorIs(t, err, dlq.ErrEmpty, "popping an empty queue must report ErrEmpty")

	base := time.Now().Add(-time.Minute).UTC()
	require.NoError(t, repo.Push(ctx, &dlq.FailedMessage{
		ID: "older", Channel: "ch", Error: "boom", FailedAt: base,
	}))
	require.NoError(t, repo.Push(ctx, &dlq.FailedMessage{
		ID: "newer", Channel: "ch", Error: "boom", FailedAt: base.Add(time.Second),
	}))

	first, err := repo.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "older", first.ID, "Pop must return the oldest failed message first")

	second, err := repo.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "newer", second.ID)

	// Both rows consumed: the guarded delete leaves the queue empty and a
	// further Pop reports ErrEmpty rather than re-delivering.
	_, err = repo.Pop(ctx)
	require.ErrorIs(t, err, dlq.ErrEmpty)
}
