package inmemory_test

import (
	"slices"
	"strconv"
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

func TestPersonalAccessTokenRepository(t *testing.T) {
	t.Parallel()

	suite.Run(t, repotesting.NewPersonalAccessTokenRepositorySuite(
		func(_ *testing.T) repositories.PersonalAccessTokenRepository {
			return inmemory.NewPersonalAccessTokenRepository()
		},
	))
}

// setupPersonalAccessTokenRepo stores the given tokens in slice order, so the
// repository hands out IDs 1..N in that same order. Save rejects an unknown
// explicit ID, so the IDs cannot be written by the fixture itself.
func setupPersonalAccessTokenRepo(
	t *testing.T,
	tokens []domain.PersonalAccessToken,
) repositories.PersonalAccessTokenRepository {
	t.Helper()

	repo := inmemory.NewPersonalAccessTokenRepository()

	for i := range tokens {
		require.NoError(t, repo.Save(t.Context(), &tokens[i]))
		require.Equal(t, uint(i+1), tokens[i].ID, "fixture relies on sequential IDs")
	}

	return repo
}

func personalAccessTokenIDs(tokens []domain.PersonalAccessToken) []uint {
	ids := make([]uint, 0, len(tokens))
	for _, token := range tokens {
		ids = append(ids, token.ID)
	}

	return ids
}

func personalAccessTokensSortFixture() []domain.PersonalAccessToken {
	return []domain.PersonalAccessToken{
		{
			TokenableType: domain.EntityTypeUser,
			TokenableID:   30,
			Name:          "ci-runner",
			Token:         "token-c",
		},
		{
			TokenableType: domain.EntityTypeNode,
			TokenableID:   10,
			Name:          "admin-key",
			Token:         "token-a",
		},
		{
			TokenableType: domain.EntityTypeRole,
			TokenableID:   40,
			Name:          "deploy",
			Token:         "token-d",
		},
		{
			TokenableType: domain.EntityTypeGame,
			TokenableID:   20,
			Name:          "backup",
			Token:         "token-b",
		},
	}
}

func TestPersonalAccessTokenRepository_SortByScalarField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		field    string
		wantAsc  []uint
		wantDesc []uint
	}{
		{
			name:     "id",
			field:    "id",
			wantAsc:  []uint{1, 2, 3, 4},
			wantDesc: []uint{4, 3, 2, 1},
		},
		{
			name:     "name",
			field:    "name",
			wantAsc:  []uint{2, 4, 1, 3},
			wantDesc: []uint{3, 1, 4, 2},
		},
		{
			// DedicatedServer < Game < User < roles lexicographically.
			name:     "tokenable_type",
			field:    "tokenable_type",
			wantAsc:  []uint{2, 4, 1, 3},
			wantDesc: []uint{3, 1, 4, 2},
		},
		{
			name:     "tokenable_id",
			field:    "tokenable_id",
			wantAsc:  []uint{2, 4, 1, 3},
			wantDesc: []uint{3, 1, 4, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupPersonalAccessTokenRepo(t, personalAccessTokensSortFixture())

			// ACT
			asc, ascErr := repo.Find(
				t.Context(), nil, []filters.Sorting{{Field: tt.field, Direction: filters.SortDirectionAsc}}, nil,
			)
			desc, descErr := repo.Find(
				t.Context(), nil, []filters.Sorting{{Field: tt.field, Direction: filters.SortDirectionDesc}}, nil,
			)

			// ASSERT
			require.NoError(t, ascErr)
			require.NoError(t, descErr)
			require.Len(t, asc, len(tt.wantAsc))
			require.Len(t, desc, len(tt.wantDesc))
			assert.Equal(t, tt.wantAsc, personalAccessTokenIDs(asc), "ascending order by %s", tt.field)
			assert.Equal(t, tt.wantDesc, personalAccessTokenIDs(desc), "descending order by %s", tt.field)
		})
	}
}

func TestPersonalAccessTokenRepository_SortByUnknownFieldFallsThroughToNextTerm(t *testing.T) {
	t.Parallel()

	// ARRANGE
	repo := setupPersonalAccessTokenRepo(t, personalAccessTokensSortFixture())

	// ACT
	tokens, err := repo.Find(t.Context(), nil, []filters.Sorting{
		{Field: "no_such_column", Direction: filters.SortDirectionAsc},
		{Field: "name", Direction: filters.SortDirectionAsc},
	}, nil)

	// ASSERT
	require.NoError(t, err)
	require.Len(t, tokens, 4)
	assert.Equal(t, []uint{2, 4, 1, 3}, personalAccessTokenIDs(tokens),
		"an unknown sort column must be ignored so the next term decides the order")
}

func TestPersonalAccessTokenRepository_SortByLastUsedAtWithMissingValues(t *testing.T) {
	t.Parallel()

	early := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)

	fixture := func() []domain.PersonalAccessToken {
		return []domain.PersonalAccessToken{
			{TokenableType: domain.EntityTypeUser, TokenableID: 1, Name: "one", Token: "token-1"},
			{TokenableType: domain.EntityTypeUser, TokenableID: 1, Name: "two", Token: "token-2"},
			{TokenableType: domain.EntityTypeUser, TokenableID: 1, Name: "three", Token: "token-3", LastUsedAt: &late},
			{TokenableType: domain.EntityTypeUser, TokenableID: 1, Name: "four", Token: "token-4", LastUsedAt: &early},
		}
	}

	tests := []struct {
		name      string
		direction filters.SortDirection
		wantIDs   []uint
	}{
		{
			name:      "ascending_puts_never_used_tokens_first",
			direction: filters.SortDirectionAsc,
			wantIDs:   []uint{1, 2, 4, 3},
		},
		{
			name:      "descending_puts_never_used_tokens_last",
			direction: filters.SortDirectionDesc,
			wantIDs:   []uint{3, 4, 1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupPersonalAccessTokenRepo(t, fixture())

			// ACT
			for range orderQueryRepeats {
				tokens, err := repo.Find(t.Context(), nil, []filters.Sorting{
					{Field: "last_used_at", Direction: tt.direction},
					{Field: "id", Direction: filters.SortDirectionAsc},
				}, nil)

				// ASSERT
				require.NoError(t, err)
				require.Len(t, tokens, 4)
				assert.Equal(t, tt.wantIDs, personalAccessTokenIDs(tokens),
					"tokens without last_used_at must group together and the id term must break their tie")
			}
		})
	}
}

func TestPersonalAccessTokenRepository_SortByCreatedAtWithMissingValues(t *testing.T) {
	t.Parallel()

	early := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)

	// setupRepo builds a repository where tokens 1 and 2 have no created_at.
	// Save stamps created_at on insert, so the value can only be cleared by a
	// follow-up update of an already stored token.
	setupRepo := func(t *testing.T) repositories.PersonalAccessTokenRepository {
		t.Helper()

		repo := setupPersonalAccessTokenRepo(t, []domain.PersonalAccessToken{
			{TokenableType: domain.EntityTypeUser, TokenableID: 1, Name: "one", Token: "token-1"},
			{TokenableType: domain.EntityTypeUser, TokenableID: 1, Name: "two", Token: "token-2"},
			{TokenableType: domain.EntityTypeUser, TokenableID: 1, Name: "three", Token: "token-3", CreatedAt: &late},
			{TokenableType: domain.EntityTypeUser, TokenableID: 1, Name: "four", Token: "token-4", CreatedAt: &early},
		})

		for _, id := range []uint{1, 2} {
			require.NoError(t, repo.Save(t.Context(), &domain.PersonalAccessToken{
				ID:            id,
				TokenableType: domain.EntityTypeUser,
				TokenableID:   1,
				Name:          "cleared",
				Token:         "token-cleared-" + strconv.FormatUint(uint64(id), 10),
			}))
		}

		return repo
	}

	tests := []struct {
		name      string
		direction filters.SortDirection
		wantIDs   []uint
	}{
		{
			name:      "ascending_puts_missing_created_at_first",
			direction: filters.SortDirectionAsc,
			wantIDs:   []uint{1, 2, 4, 3},
		},
		{
			name:      "descending_puts_missing_created_at_last",
			direction: filters.SortDirectionDesc,
			wantIDs:   []uint{3, 4, 1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupRepo(t)

			// ACT
			for range orderQueryRepeats {
				tokens, err := repo.Find(t.Context(), nil, []filters.Sorting{
					{Field: "created_at", Direction: tt.direction},
					{Field: "id", Direction: filters.SortDirectionAsc},
				}, nil)

				// ASSERT
				require.NoError(t, err)
				require.Len(t, tokens, 4)
				assert.Equal(t, tt.wantIDs, personalAccessTokenIDs(tokens),
					"tokens without created_at must group together and the id term must break their tie")
			}
		})
	}
}

func TestPersonalAccessTokenRepository_SortByEqualCreatedAt(t *testing.T) {
	t.Parallel()

	shared := time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC)

	fixture := func() []domain.PersonalAccessToken {
		return []domain.PersonalAccessToken{
			{TokenableType: domain.EntityTypeUser, TokenableID: 1, Name: "one", Token: "t-1", CreatedAt: &shared},
			{TokenableType: domain.EntityTypeUser, TokenableID: 1, Name: "two", Token: "t-2", CreatedAt: &later},
			{TokenableType: domain.EntityTypeUser, TokenableID: 1, Name: "three", Token: "t-3", CreatedAt: &shared},
		}
	}

	t.Run("tie_is_broken_by_the_next_term", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		repo := setupPersonalAccessTokenRepo(t, fixture())

		// ACT
		for range orderQueryRepeats {
			tokens, err := repo.Find(t.Context(), nil, []filters.Sorting{
				{Field: "created_at", Direction: filters.SortDirectionAsc},
				{Field: "id", Direction: filters.SortDirectionDesc},
			}, nil)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, tokens, 3)
			assert.Equal(t, []uint{3, 1, 2}, personalAccessTokenIDs(tokens),
				"tokens sharing created_at must be ordered by the next sort term")
		}
	})

	t.Run("tie_without_a_next_term_keeps_the_tied_tokens_together", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		repo := setupPersonalAccessTokenRepo(t, fixture())

		// ACT
		for range orderQueryRepeats {
			tokens, err := repo.Find(
				t.Context(), nil,
				[]filters.Sorting{{Field: "created_at", Direction: filters.SortDirectionAsc}},
				nil,
			)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, tokens, 3)
			assert.ElementsMatch(t, []uint{1, 3}, personalAccessTokenIDs(tokens)[:2],
				"the tokens sharing the older created_at must come first, in either order")
			assert.Equal(t, uint(2), tokens[2].ID, "the newest token must come last")
		}
	})
}

func TestPersonalAccessTokenRepository_SortByUpdatedAt(t *testing.T) {
	t.Parallel()

	// ARRANGE
	// Save stamps updated_at itself, so saving in sequence is the only way to
	// obtain a known chronological order.
	repo := setupPersonalAccessTokenRepo(t, []domain.PersonalAccessToken{
		{TokenableType: domain.EntityTypeUser, TokenableID: 1, Name: "one", Token: "token-1"},
		{TokenableType: domain.EntityTypeUser, TokenableID: 1, Name: "two", Token: "token-2"},
		{TokenableType: domain.EntityTypeUser, TokenableID: 1, Name: "three", Token: "token-3"},
	})

	// ACT
	for range orderQueryRepeats {
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
		assert.True(t, slices.IsSortedFunc(asc, func(a, b domain.PersonalAccessToken) int {
			return a.UpdatedAt.Compare(*b.UpdatedAt)
		}), "ascending updated_at must return the oldest token first")
		assert.True(t, slices.IsSortedFunc(desc, func(a, b domain.PersonalAccessToken) int {
			return b.UpdatedAt.Compare(*a.UpdatedAt)
		}), "descending updated_at must return the newest token first")
		assert.ElementsMatch(t, []uint{1, 2, 3}, personalAccessTokenIDs(asc), "no token may be dropped by sorting")
	}
}
