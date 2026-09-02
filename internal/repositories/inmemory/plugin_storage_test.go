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

func TestPluginStorageRepository(t *testing.T) {
	t.Parallel()

	suite.Run(t, repotesting.NewPluginStorageRepositorySuite(
		func(_ *testing.T) repositories.PluginStorageRepository {
			return inmemory.NewPluginStorageRepository()
		},
	))
}

// setupPluginStorageRepo saves the given entries in slice order, so the
// repository hands out IDs 1..N in that same order.
func setupPluginStorageRepo(
	t *testing.T,
	entries []domain.PluginStorageEntry,
) repositories.PluginStorageRepository {
	t.Helper()

	repo := inmemory.NewPluginStorageRepository()

	for i := range entries {
		require.NoError(t, repo.Save(t.Context(), &entries[i]))
		require.Equal(t, uint64(i+1), entries[i].ID, "fixture relies on sequential IDs")
	}

	return repo
}

func pluginStorageEntryIDs(entries []domain.PluginStorageEntry) []uint64 {
	ids := make([]uint64, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}

	return ids
}

func TestPluginStorageRepository_FindIntersectsPluginIDsWithIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ids       []uint64
		pluginIDs []uint64
		wantIDs   []uint64
	}{
		{
			name:      "intersection_keeps_only_entries_matching_both_filters",
			ids:       []uint64{1, 2, 3},
			pluginIDs: []uint64{100, 300},
			wantIDs:   []uint64{1, 2},
		},
		{
			name:      "single_plugin_id_narrows_the_result_to_its_entries",
			ids:       []uint64{1, 2, 3},
			pluginIDs: []uint64{500},
			wantIDs:   []uint64{3},
		},
		{
			name:      "disjoint_sets_produce_no_result",
			ids:       []uint64{3},
			pluginIDs: []uint64{100},
			wantIDs:   []uint64{},
		},
		{
			name:      "unknown_plugin_id_produces_no_result",
			ids:       []uint64{1, 2, 3},
			pluginIDs: []uint64{999},
			wantIDs:   []uint64{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupPluginStorageRepo(t, []domain.PluginStorageEntry{
				{PluginID: 100, Key: "alpha", Payload: []byte(`{"a":1}`)},
				{PluginID: 300, Key: "beta", Payload: []byte(`{"b":2}`)},
				{PluginID: 500, Key: "gamma", Payload: []byte(`{"g":3}`)},
			})

			// ACT
			entries, err := repo.Find(t.Context(), &filters.FindPluginStorage{
				IDs:       tt.ids,
				PluginIDs: tt.pluginIDs,
			}, nil, nil)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, entries, len(tt.wantIDs))
			assert.Equal(t, tt.wantIDs, pluginStorageEntryIDs(entries),
				"only the intersection of both filters must remain")
		})
	}
}
