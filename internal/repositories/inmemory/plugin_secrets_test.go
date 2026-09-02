package inmemory_test

import (
	"slices"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestPluginSecretRepository(t *testing.T) {
	t.Parallel()

	suite.Run(t, repotesting.NewPluginSecretRepositorySuite(
		func(_ *testing.T) repositories.PluginSecretRepository {
			return inmemory.NewPluginSecretRepository()
		},
	))
}

// setupPluginSecretRepo upserts the given secrets in slice order, so the
// repository hands out IDs 1..N in that same order.
func setupPluginSecretRepo(t *testing.T, secrets []domain.PluginSecret) repositories.PluginSecretRepository {
	t.Helper()

	repo := inmemory.NewPluginSecretRepository()

	for i := range secrets {
		require.NoError(t, repo.Upsert(t.Context(), &secrets[i]))
		require.Equal(t, uint64(i+1), secrets[i].ID, "fixture relies on sequential IDs")
	}

	return repo
}

func pluginSecretIDs(secrets []domain.PluginSecret) []uint64 {
	ids := make([]uint64, 0, len(secrets))
	for _, secret := range secrets {
		ids = append(ids, secret.ID)
	}

	return ids
}

func TestPluginSecretRepository_SortByCreatedAt(t *testing.T) {
	t.Parallel()

	fixture := func() []domain.PluginSecret {
		return []domain.PluginSecret{
			{
				PluginID:  domain.Uint64ID(10),
				Key:       "ckey",
				Value:     "c",
				CreatedAt: new(time.Date(2024, time.March, 1, 0, 0, 0, 0, time.UTC)),
			},
			{
				PluginID:  domain.Uint64ID(10),
				Key:       "akey",
				Value:     "a",
				CreatedAt: new(time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)),
			},
			{
				PluginID:  domain.Uint64ID(10),
				Key:       "dkey",
				Value:     "d",
				CreatedAt: new(time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC)),
			},
			{
				PluginID:  domain.Uint64ID(10),
				Key:       "bkey",
				Value:     "b",
				CreatedAt: new(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC)),
			},
		}
	}

	tests := []struct {
		name      string
		direction filters.SortDirection
		wantIDs   []uint64
	}{
		{
			name:      "ascending_returns_the_oldest_secret_first",
			direction: filters.SortDirectionAsc,
			wantIDs:   []uint64{2, 4, 1, 3},
		},
		{
			name:      "descending_returns_the_newest_secret_first",
			direction: filters.SortDirectionDesc,
			wantIDs:   []uint64{3, 1, 4, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupPluginSecretRepo(t, fixture())

			// ACT
			secrets, err := repo.Find(
				t.Context(), nil, []filters.Sorting{{Field: "created_at", Direction: tt.direction}}, nil,
			)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, secrets, 4)
			assert.Equal(t, tt.wantIDs, pluginSecretIDs(secrets), "order by created_at %s", tt.direction)
		})
	}
}

func TestPluginSecretRepository_SortByEqualCreatedAtKeepsNextTermDeciding(t *testing.T) {
	t.Parallel()

	shared := time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC)

	// ARRANGE
	repo := setupPluginSecretRepo(t, []domain.PluginSecret{
		{PluginID: domain.Uint64ID(10), Key: "akey", Value: "a", CreatedAt: &shared},
		{PluginID: domain.Uint64ID(10), Key: "bkey", Value: "b", CreatedAt: &later},
		{PluginID: domain.Uint64ID(10), Key: "ckey", Value: "c", CreatedAt: &shared},
	})

	// ACT
	secrets, err := repo.Find(t.Context(), nil, []filters.Sorting{
		{Field: "created_at", Direction: filters.SortDirectionAsc},
		{Field: "key", Direction: filters.SortDirectionDesc},
	}, nil)

	// ASSERT
	require.NoError(t, err)
	require.Len(t, secrets, 3)
	assert.Equal(t, []uint64{3, 1, 2}, pluginSecretIDs(secrets),
		"secrets sharing created_at must be ordered by the next sort term")
}

func TestPluginSecretRepository_SortByUpdatedAt(t *testing.T) {
	t.Parallel()

	// ARRANGE
	// Upsert stamps updated_at itself, so upserting in sequence is the only way
	// to obtain a known chronological order.
	repo := setupPluginSecretRepo(t, []domain.PluginSecret{
		{PluginID: domain.Uint64ID(10), Key: "akey", Value: "a"},
		{PluginID: domain.Uint64ID(10), Key: "bkey", Value: "b"},
		{PluginID: domain.Uint64ID(10), Key: "ckey", Value: "c"},
	})

	// ACT
	asc, ascErr := repo.Find(
		t.Context(), nil, []filters.Sorting{{Field: "updated_at", Direction: filters.SortDirectionAsc}}, nil,
	)
	desc, descErr := repo.Find(
		t.Context(), nil, []filters.Sorting{{Field: "updated_at", Direction: filters.SortDirectionDesc}}, nil,
	)

	// ASSERT
	require.NoError(t, ascErr)
	require.NoError(t, descErr)
	require.Len(t, asc, 3)
	require.Len(t, desc, 3)
	assert.True(t, slices.IsSortedFunc(asc, func(a, b domain.PluginSecret) int {
		return a.UpdatedAt.Compare(*b.UpdatedAt)
	}), "ascending updated_at must return the oldest secret first")
	assert.True(t, slices.IsSortedFunc(desc, func(a, b domain.PluginSecret) int {
		return b.UpdatedAt.Compare(*a.UpdatedAt)
	}), "descending updated_at must return the newest secret first")
	assert.ElementsMatch(t, []uint64{1, 2, 3}, pluginSecretIDs(asc), "no secret may be dropped by sorting")
}
