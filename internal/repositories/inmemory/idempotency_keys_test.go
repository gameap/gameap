package inmemory_test

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestIdempotencyKeyRepository(t *testing.T) {
	t.Parallel()

	suite.Run(t, repotesting.NewIdempotencyKeyRepositorySuite(
		func(_ *testing.T) repositories.IdempotencyKeyRepository {
			return inmemory.NewIdempotencyKeyRepository()
		},
	))
}

func TestIdempotencyKeyRepository_Find_returns_copy(t *testing.T) {
	t.Parallel()

	repo := inmemory.NewIdempotencyKeyRepository()
	createdAt := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	saved, err := repo.Save(context.Background(), &domain.IdempotencyKey{
		UserID:          1,
		KeyHash:         "copy",
		ResponseStatus:  200,
		ResponseHeaders: map[string][]string{"Content-Type": {"application/json"}},
		ResponseBody:    []byte(`{"status":"ok"}`),
		CreatedAt:       createdAt,
		ExpiresAt:       createdAt.Add(time.Hour),
	})
	require.NoError(t, err)
	require.True(t, saved)

	found, err := repo.Find(context.Background(), 1, "copy", createdAt)
	require.NoError(t, err)
	require.NotNil(t, found)

	found.ResponseBody[0] = 'X'
	found.ResponseHeaders["Content-Type"][0] = "text/plain"

	again, err := repo.Find(context.Background(), 1, "copy", createdAt)
	require.NoError(t, err)
	require.NotNil(t, again)
	assert.Equal(t, `{"status":"ok"}`, string(again.ResponseBody))
	assert.Equal(t, []string{"application/json"}, again.ResponseHeaders["Content-Type"])
}
