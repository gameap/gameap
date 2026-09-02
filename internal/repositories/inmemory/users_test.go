package inmemory

import (
	"slices"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestUserRepository(t *testing.T) {
	t.Parallel()

	suite.Run(t, repotesting.NewUserRepositorySuite(
		func(_ *testing.T) repositories.UserRepository {
			return NewUserRepository()
		},
	))
}

// userOrderQueryRepeats is how often an order-sensitive query is repeated. The
// repository collects its rows from a map, whose iteration order Go randomises
// per range, so a single query only exercises one of the possible input orders
// of the comparator.
const userOrderQueryRepeats = 16

// setupUserRepo stores the given users and hands back the repository through
// its public interface. The users carry explicit IDs so Save keeps the
// CreatedAt values as written instead of stamping them with time.Now().
func setupUserRepo(t *testing.T, users []domain.User) repositories.UserRepository {
	t.Helper()

	repo := NewUserRepository()

	for i := range users {
		require.NoError(t, repo.Save(t.Context(), &users[i]))
	}

	return repo
}

func userIDs(users []domain.User) []uint {
	ids := make([]uint, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}

	return ids
}

func usersSortFixture() []domain.User {
	return []domain.User{
		{ID: 1, Login: "charlie", Email: "m@c.example", Name: new("Mia")},
		{ID: 2, Login: "alice", Email: "z@a.example"},
		{ID: 3, Login: "delta", Email: "a@d.example", Name: new("Ana")},
		{ID: 4, Login: "bravo", Email: "k@b.example", Name: new("Zoe")},
	}
}

func TestUserRepository_SortByScalarField(t *testing.T) {
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
			name:     "login",
			field:    "login",
			wantAsc:  []uint{2, 4, 1, 3},
			wantDesc: []uint{3, 1, 4, 2},
		},
		{
			name:     "email",
			field:    "email",
			wantAsc:  []uint{3, 4, 1, 2},
			wantDesc: []uint{2, 1, 4, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupUserRepo(t, usersSortFixture())

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
			assert.Equal(t, tt.wantAsc, userIDs(asc), "ascending order by %s", tt.field)
			assert.Equal(t, tt.wantDesc, userIDs(desc), "descending order by %s", tt.field)
		})
	}
}

func TestUserRepository_SortByUnknownFieldFallsThroughToNextTerm(t *testing.T) {
	t.Parallel()

	// ARRANGE
	repo := setupUserRepo(t, usersSortFixture())

	// ACT
	users, err := repo.Find(t.Context(), nil, []filters.Sorting{
		{Field: "no_such_column", Direction: filters.SortDirectionAsc},
		{Field: "login", Direction: filters.SortDirectionAsc},
	}, nil)

	// ASSERT
	require.NoError(t, err)
	require.Len(t, users, 4)
	assert.Equal(t, []uint{2, 4, 1, 3}, userIDs(users),
		"an unknown sort column must tie so the next term decides the order")
}

func TestUserRepository_SortByNullableColumns(t *testing.T) {
	t.Parallel()

	early := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)

	nameFixture := func() []domain.User {
		return []domain.User{
			{ID: 1, Login: "one", Email: "one@example.com"},
			{ID: 2, Login: "two", Email: "two@example.com"},
			{ID: 3, Login: "three", Email: "three@example.com", Name: new("Zoe")},
			{ID: 4, Login: "four", Email: "four@example.com", Name: new("Ana")},
		}
	}

	createdAtFixture := func() []domain.User {
		return []domain.User{
			{ID: 1, Login: "one", Email: "one@example.com"},
			{ID: 2, Login: "two", Email: "two@example.com"},
			{ID: 3, Login: "three", Email: "three@example.com", CreatedAt: &late},
			{ID: 4, Login: "four", Email: "four@example.com", CreatedAt: &early},
		}
	}

	tests := []struct {
		name      string
		field     string
		users     []domain.User
		direction filters.SortDirection
		wantIDs   []uint
	}{
		{
			name:      "name_ascending_puts_missing_value_first",
			field:     "name",
			users:     nameFixture(),
			direction: filters.SortDirectionAsc,
			wantIDs:   []uint{1, 2, 4, 3},
		},
		{
			name:      "name_descending_puts_missing_value_last",
			field:     "name",
			users:     nameFixture(),
			direction: filters.SortDirectionDesc,
			wantIDs:   []uint{3, 4, 1, 2},
		},
		{
			name:      "created_at_ascending_puts_missing_value_first",
			field:     "created_at",
			users:     createdAtFixture(),
			direction: filters.SortDirectionAsc,
			wantIDs:   []uint{1, 2, 4, 3},
		},
		{
			name:      "created_at_descending_puts_missing_value_last",
			field:     "created_at",
			users:     createdAtFixture(),
			direction: filters.SortDirectionDesc,
			wantIDs:   []uint{3, 4, 1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupUserRepo(t, tt.users)

			// ACT
			// Repeated because the repository collects the rows from a map, so
			// every query hands the comparator a different starting order.
			for range userOrderQueryRepeats {
				users, err := repo.Find(t.Context(), nil, []filters.Sorting{
					{Field: tt.field, Direction: tt.direction},
					{Field: "id", Direction: filters.SortDirectionAsc},
				}, nil)

				// ASSERT
				require.NoError(t, err)
				require.Len(t, users, 4)
				assert.Equal(t, tt.wantIDs, userIDs(users),
					"users without %s must group together and the id term must break their tie", tt.field)
			}
		})
	}
}

func TestUserRepository_SortByEqualCreatedAtKeepsNextTermDeciding(t *testing.T) {
	t.Parallel()

	shared := time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC)

	// ARRANGE
	repo := setupUserRepo(t, []domain.User{
		{ID: 1, Login: "one", Email: "one@example.com", CreatedAt: &shared},
		{ID: 2, Login: "two", Email: "two@example.com", CreatedAt: &later},
		{ID: 3, Login: "three", Email: "three@example.com", CreatedAt: &shared},
	})

	// ACT
	for range userOrderQueryRepeats {
		users, err := repo.Find(t.Context(), nil, []filters.Sorting{
			{Field: "created_at", Direction: filters.SortDirectionAsc},
			{Field: "id", Direction: filters.SortDirectionDesc},
		}, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, users, 3)
		assert.Equal(t, []uint{3, 1, 2}, userIDs(users),
			"users sharing created_at must be ordered by the next sort term")
	}
}

func TestUserRepository_SortByUpdatedAt(t *testing.T) {
	t.Parallel()

	// ARRANGE
	// Save stamps updated_at itself, so saving in sequence is the only way to
	// obtain a known chronological order.
	repo := setupUserRepo(t, []domain.User{
		{ID: 1, Login: "one", Email: "one@example.com"},
		{ID: 2, Login: "two", Email: "two@example.com"},
		{ID: 3, Login: "three", Email: "three@example.com"},
	})

	// ACT
	for range userOrderQueryRepeats {
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
		assert.True(t, slices.IsSortedFunc(asc, func(a, b domain.User) int {
			return a.UpdatedAt.Compare(*b.UpdatedAt)
		}), "ascending updated_at must return the oldest user first")
		assert.True(t, slices.IsSortedFunc(desc, func(a, b domain.User) int {
			return b.UpdatedAt.Compare(*a.UpdatedAt)
		}), "descending updated_at must return the newest user first")
		assert.ElementsMatch(t, []uint{1, 2, 3}, userIDs(asc), "no user may be dropped by sorting")
	}
}
