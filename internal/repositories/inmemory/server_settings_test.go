package inmemory_test

import (
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestServerSettingRepository(t *testing.T) {
	t.Parallel()

	suite.Run(t, repotesting.NewServerSettingRepositorySuite(
		func(_ *testing.T) repositories.ServerSettingRepository {
			return inmemory.NewServerSettingRepository()
		},
	))
}

func setupServerSettingRepo(t *testing.T, settings []domain.ServerSetting) repositories.ServerSettingRepository {
	t.Helper()

	repo := inmemory.NewServerSettingRepository()

	for i := range settings {
		require.NoError(t, repo.Save(t.Context(), &settings[i]))
	}

	return repo
}

func serverSettingIDs(settings []domain.ServerSetting) []uint {
	ids := make([]uint, 0, len(settings))
	for _, setting := range settings {
		ids = append(ids, setting.ID)
	}

	return ids
}

func serverSettingsSortFixture() []domain.ServerSetting {
	return []domain.ServerSetting{
		{ID: 1, Name: "rcon", ServerID: 30, Value: domain.NewServerSettingValue("secret")},
		{ID: 2, Name: "autostart", ServerID: 10, Value: domain.NewServerSettingValue(true)},
		{ID: 3, Name: "slots", ServerID: 40, Value: domain.NewServerSettingValue(16)},
		{ID: 4, Name: "map", ServerID: 20, Value: domain.NewServerSettingValue("de_dust2")},
	}
}

func TestServerSettingRepository_SortByScalarField(t *testing.T) {
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
			name:     "server_id",
			field:    "server_id",
			wantAsc:  []uint{2, 4, 1, 3},
			wantDesc: []uint{3, 1, 4, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupServerSettingRepo(t, serverSettingsSortFixture())

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
			assert.Equal(t, tt.wantAsc, serverSettingIDs(asc), "ascending order by %s", tt.field)
			assert.Equal(t, tt.wantDesc, serverSettingIDs(desc), "descending order by %s", tt.field)
		})
	}
}

func TestServerSettingRepository_SortByUnknownFieldFallsThroughToNextTerm(t *testing.T) {
	t.Parallel()

	// ARRANGE
	repo := setupServerSettingRepo(t, serverSettingsSortFixture())

	// ACT
	settings, err := repo.Find(t.Context(), nil, []filters.Sorting{
		{Field: "no_such_column", Direction: filters.SortDirectionAsc},
		{Field: "name", Direction: filters.SortDirectionAsc},
	}, nil)

	// ASSERT
	require.NoError(t, err)
	require.Len(t, settings, 4)
	assert.Equal(t, []uint{2, 4, 1, 3}, serverSettingIDs(settings),
		"an unknown sort column must tie so the next term decides the order")
}

func TestServerSettingRepository_FindIntersectsServerIDsWithIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ids       []uint
		serverIDs []uint
		wantIDs   []uint
	}{
		{
			name:      "intersection_keeps_only_settings_matching_both_filters",
			ids:       []uint{1, 2, 3},
			serverIDs: []uint{100, 300},
			wantIDs:   []uint{1, 2},
		},
		{
			name:      "single_server_id_narrows_the_result_to_its_settings",
			ids:       []uint{1, 2, 3},
			serverIDs: []uint{500},
			wantIDs:   []uint{3},
		},
		{
			name:      "disjoint_sets_produce_no_result",
			ids:       []uint{3},
			serverIDs: []uint{100},
			wantIDs:   []uint{},
		},
		{
			name:      "unknown_server_id_produces_no_result",
			ids:       []uint{1, 2, 3},
			serverIDs: []uint{999},
			wantIDs:   []uint{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupServerSettingRepo(t, []domain.ServerSetting{
				{ID: 1, Name: "autostart", ServerID: 100, Value: domain.NewServerSettingValue(true)},
				{ID: 2, Name: "slots", ServerID: 300, Value: domain.NewServerSettingValue(16)},
				{ID: 3, Name: "rcon", ServerID: 500, Value: domain.NewServerSettingValue("secret")},
			})

			// ACT
			settings, err := repo.Find(t.Context(), &filters.FindServerSetting{
				IDs:       tt.ids,
				ServerIDs: tt.serverIDs,
			}, []filters.Sorting{{Field: "id", Direction: filters.SortDirectionAsc}}, nil)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, settings, len(tt.wantIDs))
			assert.Equal(t, tt.wantIDs, serverSettingIDs(settings),
				"only the intersection of both filters must remain")
		})
	}
}
