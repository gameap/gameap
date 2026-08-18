package testing

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type PluginStorageRepositorySuite struct {
	suite.Suite

	repo repositories.PluginStorageRepository
	fn   func(t *testing.T) repositories.PluginStorageRepository
}

func NewPluginStorageRepositorySuite(
	fn func(t *testing.T) repositories.PluginStorageRepository,
) *PluginStorageRepositorySuite {
	return &PluginStorageRepositorySuite{
		fn: fn,
	}
}

func (s *PluginStorageRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

func (s *PluginStorageRepositorySuite) TestPluginStorageRepositorySave() {
	ctx := context.Background()

	s.T().Run("insert_new_entry", func(t *testing.T) {
		entry := &domain.PluginStorageEntry{
			PluginID: 1,
			Key:      "config",
			Payload:  []byte(`{"setting": "value"}`),
		}

		err := s.repo.Save(ctx, entry)
		require.NoError(t, err)
		assert.NotZero(t, entry.ID)
		assert.NotNil(t, entry.CreatedAt)
		assert.NotNil(t, entry.UpdatedAt)

		filter := &filters.FindPluginStorage{IDs: []uint64{entry.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, entry.ID, results[0].ID)
		assert.Equal(t, uint64(1), results[0].PluginID)
		assert.Equal(t, "config", results[0].Key)
		assert.Nil(t, results[0].EntityType)
		assert.Nil(t, results[0].EntityID)
		assert.Equal(t, []byte(`{"setting": "value"}`), results[0].Payload)
		assert.NotNil(t, results[0].CreatedAt)
		assert.NotNil(t, results[0].UpdatedAt)
	})

	s.T().Run("insert_entry_with_entity", func(t *testing.T) {
		entry := &domain.PluginStorageEntry{
			PluginID:   2,
			Key:        "server_config",
			EntityType: new("server"),
			EntityID:   new(uint(100)),
			Payload:    []byte(`{"port": 27015}`),
		}

		err := s.repo.Save(ctx, entry)
		require.NoError(t, err)
		assert.NotZero(t, entry.ID)

		filter := &filters.FindPluginStorage{IDs: []uint64{entry.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, entry.ID, results[0].ID)
		assert.Equal(t, uint64(2), results[0].PluginID)
		assert.Equal(t, "server_config", results[0].Key)
		require.NotNil(t, results[0].EntityType)
		assert.Equal(t, "server", *results[0].EntityType)
		require.NotNil(t, results[0].EntityID)
		assert.Equal(t, uint(100), *results[0].EntityID)
		assert.Equal(t, []byte(`{"port": 27015}`), results[0].Payload)
	})

	s.T().Run("update_existing_entry", func(t *testing.T) {
		entry := &domain.PluginStorageEntry{
			PluginID: 3,
			Key:      "cache",
			Payload:  []byte(`{"data": "initial"}`),
		}

		err := s.repo.Save(ctx, entry)
		require.NoError(t, err)
		originalID := entry.ID
		originalCreatedAt := entry.CreatedAt

		time.Sleep(10 * time.Millisecond)

		entry.Payload = []byte(`{"data": "updated"}`)
		err = s.repo.Save(ctx, entry)
		require.NoError(t, err)
		assert.Equal(t, originalID, entry.ID)

		filter := &filters.FindPluginStorage{IDs: []uint64{entry.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, []byte(`{"data": "updated"}`), results[0].Payload)
		assert.InDelta(t, originalCreatedAt.Unix(), results[0].CreatedAt.Unix(), 1.0)
		assert.GreaterOrEqual(t, results[0].UpdatedAt.Unix(), originalCreatedAt.Unix())
	})

	s.T().Run("save_multiple_keys_same_plugin", func(t *testing.T) {
		pluginID := uint64(10)

		entry1 := &domain.PluginStorageEntry{
			PluginID: pluginID,
			Key:      "key1",
			Payload:  []byte(`{"k": "v1"}`),
		}
		entry2 := &domain.PluginStorageEntry{
			PluginID: pluginID,
			Key:      "key2",
			Payload:  []byte(`{"k": "v2"}`),
		}

		require.NoError(t, s.repo.Save(ctx, entry1))
		require.NoError(t, s.repo.Save(ctx, entry2))
		assert.NotEqual(t, entry1.ID, entry2.ID)

		filter := &filters.FindPluginStorage{PluginIDs: []uint64{pluginID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	s.T().Run("save_same_key_different_entities", func(t *testing.T) {
		pluginID := uint64(20)

		entry1 := &domain.PluginStorageEntry{
			PluginID:   pluginID,
			Key:        "stats",
			EntityType: new("server"),
			EntityID:   new(uint(1)),
			Payload:    []byte(`{"cpu": 50}`),
		}
		entry2 := &domain.PluginStorageEntry{
			PluginID:   pluginID,
			Key:        "stats",
			EntityType: new("server"),
			EntityID:   new(uint(2)),
			Payload:    []byte(`{"cpu": 75}`),
		}

		require.NoError(t, s.repo.Save(ctx, entry1))
		require.NoError(t, s.repo.Save(ctx, entry2))
		assert.NotEqual(t, entry1.ID, entry2.ID)

		filter := &filters.FindPluginStorage{
			PluginIDs: []uint64{pluginID},
			Keys:      []string{"stats"},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})
}

// A plugin's storage_set for a key it already holds is a fresh entry (no ID)
// each time: the second save must land on the same row, for a global key
// (NULL entity, which no unique index can guard) as much as for a scoped one.
func (s *PluginStorageRepositorySuite) TestPluginStorageRepositorySaveAgain() {
	ctx := context.Background()

	s.T().Run("global_key_is_updated_in_place", func(t *testing.T) {
		pluginID := uint64(900)

		require.NoError(t, s.repo.Save(ctx, &domain.PluginStorageEntry{
			PluginID: pluginID,
			Key:      "rsp:account",
			Payload:  []byte(`{"state":"registering"}`),
		}))
		second := &domain.PluginStorageEntry{
			PluginID: pluginID,
			Key:      "rsp:account",
			Payload:  []byte(`{"state":"active"}`),
		}
		require.NoError(t, s.repo.Save(ctx, second))
		assert.NotZero(t, second.ID, "the save reports the row it landed on")

		filter := &filters.FindPluginStorage{
			PluginIDs: []uint64{pluginID},
			Keys:      []string{"rsp:account"},
			EntityPairs: []domain.PluginStorageEntityPair{
				{EntityType: nil, EntityID: nil},
			},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1, "a second save must not add a second row")
		assert.Equal(t, []byte(`{"state":"active"}`), results[0].Payload)
		assert.Equal(t, second.ID, results[0].ID)
	})

	s.T().Run("scoped_key_is_updated_in_place", func(t *testing.T) {
		pluginID := uint64(901)

		require.NoError(t, s.repo.Save(ctx, &domain.PluginStorageEntry{
			PluginID:   pluginID,
			Key:        "rsp:node",
			EntityType: new("node"),
			EntityID:   new(uint(1)),
			Payload:    []byte(`{"cli":"missing"}`),
		}))
		require.NoError(t, s.repo.Save(ctx, &domain.PluginStorageEntry{
			PluginID:   pluginID,
			Key:        "rsp:node",
			EntityType: new("node"),
			EntityID:   new(uint(1)),
			Payload:    []byte(`{"cli":"installed"}`),
		}))

		filter := &filters.FindPluginStorage{
			PluginIDs: []uint64{pluginID},
			Keys:      []string{"rsp:node"},
			EntityPairs: []domain.PluginStorageEntityPair{
				{EntityType: new("node"), EntityID: new(uint(1))},
			},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, []byte(`{"cli":"installed"}`), results[0].Payload)
	})

	s.T().Run("global_keys_stay_apart", func(t *testing.T) {
		pluginID := uint64(902)

		require.NoError(t, s.repo.Save(ctx, &domain.PluginStorageEntry{PluginID: pluginID, Key: "a", Payload: []byte(`1`)}))
		require.NoError(t, s.repo.Save(ctx, &domain.PluginStorageEntry{PluginID: pluginID, Key: "b", Payload: []byte(`2`)}))
		require.NoError(t, s.repo.Save(ctx, &domain.PluginStorageEntry{PluginID: pluginID + 1, Key: "a", Payload: []byte(`3`)}))
		require.NoError(t, s.repo.Save(ctx, &domain.PluginStorageEntry{PluginID: pluginID, Key: "a", Payload: []byte(`4`)}))

		results, err := s.repo.Find(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{pluginID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 2)
		byKey := map[string][]byte{}
		for _, r := range results {
			byKey[r.Key] = r.Payload
		}
		assert.Equal(t, []byte(`4`), byKey["a"])
		assert.Equal(t, []byte(`2`), byKey["b"])

		other, err := s.repo.Find(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{pluginID + 1}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, other, 1)
		assert.Equal(t, []byte(`3`), other[0].Payload)
	})
}

func (s *PluginStorageRepositorySuite) TestPluginStorageRepositoryFind() {
	ctx := context.Background()

	entry1 := &domain.PluginStorageEntry{
		PluginID: 100,
		Key:      "config",
		Payload:  []byte(`{"a": 1}`),
	}
	entry2 := &domain.PluginStorageEntry{
		PluginID:   100,
		Key:        "data",
		EntityType: new("server"),
		EntityID:   new(uint(50)),
		Payload:    []byte(`{"b": 2}`),
	}
	entry3 := &domain.PluginStorageEntry{
		PluginID: 200,
		Key:      "config",
		Payload:  []byte(`{"c": 3}`),
	}

	require.NoError(s.T(), s.repo.Save(ctx, entry1))
	require.NoError(s.T(), s.repo.Save(ctx, entry2))
	require.NoError(s.T(), s.repo.Save(ctx, entry3))

	s.T().Run("find_by_id", func(t *testing.T) {
		filter := &filters.FindPluginStorage{
			IDs: []uint64{entry1.ID},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, entry1.ID, results[0].ID)
		assert.Equal(t, "config", results[0].Key)
	})

	s.T().Run("find_by_multiple_ids", func(t *testing.T) {
		filter := &filters.FindPluginStorage{
			IDs: []uint64{entry1.ID, entry3.ID},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 2)

		ids := []uint64{results[0].ID, results[1].ID}
		assert.Contains(t, ids, entry1.ID)
		assert.Contains(t, ids, entry3.ID)
	})

	s.T().Run("find_by_plugin_id", func(t *testing.T) {
		filter := &filters.FindPluginStorage{
			PluginIDs: []uint64{100},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 2)

		for _, result := range results {
			assert.Equal(t, uint64(100), result.PluginID)
		}
	})

	s.T().Run("find_by_plugin_and_key", func(t *testing.T) {
		filter := &filters.FindPluginStorage{
			PluginIDs: []uint64{100},
			Keys:      []string{"config"},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "config", results[0].Key)
		assert.Equal(t, uint64(100), results[0].PluginID)
	})

	s.T().Run("find_by_entity_pair", func(t *testing.T) {
		filter := &filters.FindPluginStorage{
			EntityPairs: []domain.PluginStorageEntityPair{
				{EntityType: new("server"), EntityID: new(uint(50))},
			},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, entry2.ID, results[0].ID)
	})

	s.T().Run("find_with_nil_entity_pair", func(t *testing.T) {
		filter := &filters.FindPluginStorage{
			PluginIDs: []uint64{100},
			EntityPairs: []domain.PluginStorageEntityPair{
				{EntityType: nil, EntityID: nil},
			},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, entry1.ID, results[0].ID)
		assert.Nil(t, results[0].EntityType)
		assert.Nil(t, results[0].EntityID)
	})

	s.T().Run("find_with_nil_filter", func(t *testing.T) {
		results, err := s.repo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	s.T().Run("find_non_existent", func(t *testing.T) {
		filter := &filters.FindPluginStorage{
			IDs: []uint64{99999},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("find_with_pagination", func(t *testing.T) {
		filter := &filters.FindPluginStorage{
			PluginIDs: []uint64{100},
		}
		pagination := &filters.Pagination{
			Limit:  1,
			Offset: 0,
		}

		results, err := s.repo.Find(ctx, filter, nil, pagination)
		require.NoError(t, err)
		require.Len(t, results, 1)
	})

	s.T().Run("find_with_zero_limit_uses_default", func(t *testing.T) {
		pagination := &filters.Pagination{
			Limit:  0,
			Offset: 0,
		}

		results, err := s.repo.Find(ctx, nil, nil, pagination)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(results), int(filters.DefaultLimit))
	})

	s.T().Run("find_with_order_desc", func(t *testing.T) {
		order := []filters.Sorting{
			{Field: "id", Direction: filters.SortDirectionDesc},
		}

		results, err := s.repo.Find(ctx, nil, order, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 2)

		for i := 0; i < len(results)-1; i++ {
			assert.GreaterOrEqual(t, results[i].ID, results[i+1].ID)
		}
	})
}

func (s *PluginStorageRepositorySuite) TestPluginStorageRepositoryDelete() {
	ctx := context.Background()

	s.T().Run("delete_existing_entry", func(t *testing.T) {
		entry := &domain.PluginStorageEntry{
			PluginID: 300,
			Key:      "delete_me",
			Payload:  []byte(`{}`),
		}

		require.NoError(t, s.repo.Save(ctx, entry))
		entryID := entry.ID

		err := s.repo.Delete(ctx, entryID)
		require.NoError(t, err)

		filter := &filters.FindPluginStorage{
			IDs: []uint64{entryID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("delete_non_existent", func(t *testing.T) {
		err := s.repo.Delete(ctx, 99999)
		require.NoError(t, err)
	})
}

func (s *PluginStorageRepositorySuite) TestPluginStorageRepositoryDeleteByPlugin() {
	ctx := context.Background()

	s.T().Run("delete_all_plugin_entries", func(t *testing.T) {
		pluginID := uint64(400)

		entry1 := &domain.PluginStorageEntry{
			PluginID: pluginID,
			Key:      "key1",
			Payload:  []byte(`{}`),
		}
		entry2 := &domain.PluginStorageEntry{
			PluginID: pluginID,
			Key:      "key2",
			Payload:  []byte(`{}`),
		}

		require.NoError(t, s.repo.Save(ctx, entry1))
		require.NoError(t, s.repo.Save(ctx, entry2))

		err := s.repo.DeleteByPlugin(ctx, pluginID)
		require.NoError(t, err)

		filter := &filters.FindPluginStorage{
			PluginIDs: []uint64{pluginID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("delete_by_empty_filter_is_rejected", func(t *testing.T) {
		entry := &domain.PluginStorageEntry{PluginID: 600, Key: "keep", Payload: []byte(`{}`)}
		require.NoError(t, s.repo.Save(ctx, entry))

		// An empty filter must not wipe the table.
		err := s.repo.DeleteByFilter(ctx, &filters.FindPluginStorage{})
		require.Error(t, err, "an all-empty filter must be rejected, not delete everything")

		results, err := s.repo.Find(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{600}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1, "the entry must survive a rejected empty-filter delete")
	})

	s.T().Run("delete_by_nil_filter_is_rejected", func(t *testing.T) {
		// ARRANGE — a lone entry on an isolated plugin id.
		entry := &domain.PluginStorageEntry{PluginID: 615, Key: "keep_nil", Payload: []byte(`{}`)}
		require.NoError(t, s.repo.Save(ctx, entry))

		// ACT — a nil filter must be rejected, not treated as "match everything".
		err := s.repo.DeleteByFilter(ctx, nil)

		// ASSERT
		require.Error(t, err, "a nil filter must be rejected, not delete everything")
		assert.Contains(t, err.Error(), "non-empty filter is required",
			"the rejection must name the missing-filter contract")

		results, err := s.repo.Find(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{615}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1, "the entry must survive a rejected nil-filter delete")
	})

	s.T().Run("delete_preserves_other_plugins", func(t *testing.T) {
		pluginID1 := uint64(500)
		pluginID2 := uint64(501)

		entry1 := &domain.PluginStorageEntry{
			PluginID: pluginID1,
			Key:      "config",
			Payload:  []byte(`{}`),
		}
		entry2 := &domain.PluginStorageEntry{
			PluginID: pluginID2,
			Key:      "config",
			Payload:  []byte(`{}`),
		}

		require.NoError(t, s.repo.Save(ctx, entry1))
		require.NoError(t, s.repo.Save(ctx, entry2))

		err := s.repo.DeleteByPlugin(ctx, pluginID1)
		require.NoError(t, err)

		filter := &filters.FindPluginStorage{
			PluginIDs: []uint64{pluginID2},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, pluginID2, results[0].PluginID)
	})
}

func (s *PluginStorageRepositorySuite) TestPluginStorageRepositoryIntegration() {
	ctx := context.Background()

	s.T().Run("full_lifecycle", func(t *testing.T) {
		pluginID := uint64(600)

		entry := &domain.PluginStorageEntry{
			PluginID:   pluginID,
			Key:        "lifecycle_test",
			EntityType: new("node"),
			EntityID:   new(uint(42)),
			Payload:    []byte(`{"stage": "create"}`),
		}

		err := s.repo.Save(ctx, entry)
		require.NoError(t, err)
		assert.NotZero(t, entry.ID)

		filter := &filters.FindPluginStorage{
			IDs: []uint64{entry.ID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "lifecycle_test", results[0].Key)
		assert.Equal(t, []byte(`{"stage": "create"}`), results[0].Payload)
		require.NotNil(t, results[0].EntityType)
		assert.Equal(t, "node", *results[0].EntityType)
		require.NotNil(t, results[0].EntityID)
		assert.Equal(t, uint(42), *results[0].EntityID)

		entry.Payload = []byte(`{"stage": "update"}`)
		err = s.repo.Save(ctx, entry)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, []byte(`{"stage": "update"}`), results[0].Payload)

		err = s.repo.Delete(ctx, entry.ID)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("multiple_plugins_isolation", func(t *testing.T) {
		plugin1ID := uint64(700)
		plugin2ID := uint64(701)

		entries := []*domain.PluginStorageEntry{
			{PluginID: plugin1ID, Key: "config", Payload: []byte(`{"p": 1}`)},
			{PluginID: plugin1ID, Key: "cache", Payload: []byte(`{"p": 1}`)},
			{PluginID: plugin2ID, Key: "config", Payload: []byte(`{"p": 2}`)},
			{PluginID: plugin2ID, Key: "state", Payload: []byte(`{"p": 2}`)},
		}

		for _, entry := range entries {
			require.NoError(t, s.repo.Save(ctx, entry))
		}

		filter1 := &filters.FindPluginStorage{PluginIDs: []uint64{plugin1ID}}
		results1, err := s.repo.Find(ctx, filter1, nil, nil)
		require.NoError(t, err)
		require.Len(t, results1, 2)
		for _, r := range results1 {
			assert.Equal(t, plugin1ID, r.PluginID)
		}

		filter2 := &filters.FindPluginStorage{PluginIDs: []uint64{plugin2ID}}
		results2, err := s.repo.Find(ctx, filter2, nil, nil)
		require.NoError(t, err)
		require.Len(t, results2, 2)
		for _, r := range results2 {
			assert.Equal(t, plugin2ID, r.PluginID)
		}

		err = s.repo.DeleteByPlugin(ctx, plugin1ID)
		require.NoError(t, err)

		results1, err = s.repo.Find(ctx, filter1, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results1)

		results2, err = s.repo.Find(ctx, filter2, nil, nil)
		require.NoError(t, err)
		require.Len(t, results2, 2)
	})

	s.T().Run("entity_association", func(t *testing.T) {
		pluginID := uint64(800)

		entryGlobal := &domain.PluginStorageEntry{
			PluginID: pluginID,
			Key:      "global_config",
			Payload:  []byte(`{"global": true}`),
		}
		entryServer1 := &domain.PluginStorageEntry{
			PluginID:   pluginID,
			Key:        "server_config",
			EntityType: new("server"),
			EntityID:   new(uint(1)),
			Payload:    []byte(`{"server": 1}`),
		}
		entryServer2 := &domain.PluginStorageEntry{
			PluginID:   pluginID,
			Key:        "server_config",
			EntityType: new("server"),
			EntityID:   new(uint(2)),
			Payload:    []byte(`{"server": 2}`),
		}

		require.NoError(t, s.repo.Save(ctx, entryGlobal))
		require.NoError(t, s.repo.Save(ctx, entryServer1))
		require.NoError(t, s.repo.Save(ctx, entryServer2))

		filterGlobal := &filters.FindPluginStorage{
			PluginIDs: []uint64{pluginID},
			EntityPairs: []domain.PluginStorageEntityPair{
				{EntityType: nil, EntityID: nil},
			},
		}
		resultsGlobal, err := s.repo.Find(ctx, filterGlobal, nil, nil)
		require.NoError(t, err)
		require.Len(t, resultsGlobal, 1)
		assert.Equal(t, "global_config", resultsGlobal[0].Key)

		filterServer := &filters.FindPluginStorage{
			PluginIDs: []uint64{pluginID},
			Keys:      []string{"server_config"},
			EntityPairs: []domain.PluginStorageEntityPair{
				{EntityType: new("server"), EntityID: new(uint(1))},
			},
		}
		resultsServer, err := s.repo.Find(ctx, filterServer, nil, nil)
		require.NoError(t, err)
		require.Len(t, resultsServer, 1)
		assert.Equal(t, []byte(`{"server": 1}`), resultsServer[0].Payload)

		filterAll := &filters.FindPluginStorage{
			PluginIDs: []uint64{pluginID},
		}
		resultsAll, err := s.repo.Find(ctx, filterAll, nil, nil)
		require.NoError(t, err)
		require.Len(t, resultsAll, 3)
	})
}

func (s *PluginStorageRepositorySuite) TestPluginStorageRepositoryDeleteByFilter() {
	ctx := context.Background()

	s.T().Run("delete_by_plugin_ids", func(t *testing.T) {
		// ARRANGE
		require.NoError(t, s.repo.Save(ctx, &domain.PluginStorageEntry{PluginID: 900, Key: "a", Payload: []byte(`{}`)}))
		require.NoError(t, s.repo.Save(ctx, &domain.PluginStorageEntry{PluginID: 900, Key: "b", Payload: []byte(`{}`)}))
		require.NoError(t, s.repo.Save(ctx, &domain.PluginStorageEntry{PluginID: 901, Key: "a", Payload: []byte(`{}`)}))

		// ACT
		err := s.repo.DeleteByFilter(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{900}})

		// ASSERT
		require.NoError(t, err)

		deleted, err := s.repo.Find(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{900}}, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, deleted, "entries of plugin 900 must be deleted")

		survived, err := s.repo.Find(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{901}}, nil, nil)
		require.NoError(t, err)
		assert.Len(t, survived, 1, "entries of other plugins must survive")
	})

	s.T().Run("delete_by_plugin_and_keys", func(t *testing.T) {
		// ARRANGE
		require.NoError(t, s.repo.Save(ctx, &domain.PluginStorageEntry{PluginID: 902, Key: "remove", Payload: []byte(`{}`)}))
		require.NoError(t, s.repo.Save(ctx, &domain.PluginStorageEntry{PluginID: 902, Key: "keep", Payload: []byte(`{}`)}))

		// ACT
		err := s.repo.DeleteByFilter(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{902}, Keys: []string{"remove"}})

		// ASSERT
		require.NoError(t, err)

		remaining, err := s.repo.Find(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{902}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, remaining, 1, "only the non-matching key must survive")
		assert.Equal(t, "keep", remaining[0].Key)
	})

	s.T().Run("delete_by_entity_pairs", func(t *testing.T) {
		// ARRANGE
		require.NoError(t, s.repo.Save(ctx, &domain.PluginStorageEntry{
			PluginID: 903, Key: "stats", EntityType: new("server"), EntityID: new(uint(1)), Payload: []byte(`{}`),
		}))
		require.NoError(t, s.repo.Save(ctx, &domain.PluginStorageEntry{
			PluginID: 903, Key: "stats", EntityType: new("server"), EntityID: new(uint(2)), Payload: []byte(`{}`),
		}))

		// ACT
		err := s.repo.DeleteByFilter(ctx, &filters.FindPluginStorage{
			EntityPairs: []domain.PluginStorageEntityPair{
				{EntityType: new("server"), EntityID: new(uint(1))},
			},
		})

		// ASSERT
		require.NoError(t, err)

		remaining, err := s.repo.Find(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{903}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, remaining, 1, "only the targeted entity pair must be deleted")
		require.NotNil(t, remaining[0].EntityID)
		assert.Equal(t, uint(2), *remaining[0].EntityID)
	})

	s.T().Run("delete_by_ids", func(t *testing.T) {
		// ARRANGE
		entry := &domain.PluginStorageEntry{PluginID: 904, Key: "target", Payload: []byte(`{}`)}
		require.NoError(t, s.repo.Save(ctx, entry))
		other := &domain.PluginStorageEntry{PluginID: 904, Key: "other", Payload: []byte(`{}`)}
		require.NoError(t, s.repo.Save(ctx, other))

		// ACT
		err := s.repo.DeleteByFilter(ctx, &filters.FindPluginStorage{IDs: []uint64{entry.ID}})

		// ASSERT
		require.NoError(t, err)

		remaining, err := s.repo.Find(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{904}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, remaining, 1)
		assert.Equal(t, other.ID, remaining[0].ID)
	})

	s.T().Run("delete_with_no_matches_keeps_everything", func(t *testing.T) {
		// ARRANGE
		require.NoError(t, s.repo.Save(ctx, &domain.PluginStorageEntry{PluginID: 905, Key: "keep", Payload: []byte(`{}`)}))

		// ACT
		err := s.repo.DeleteByFilter(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{999999}})

		// ASSERT
		require.NoError(t, err)

		remaining, err := s.repo.Find(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{905}}, nil, nil)
		require.NoError(t, err)
		assert.Len(t, remaining, 1, "a no-match DeleteByFilter must not delete anything")
	})
}

func (s *PluginStorageRepositorySuite) TestPluginStorageRepositorySorting() {
	ctx := context.Background()

	// ARRANGE — one fixture for the whole method: subtests share the repo.
	entryA := &domain.PluginStorageEntry{PluginID: 950, Key: "beta", Payload: []byte(`{}`)}
	entryB := &domain.PluginStorageEntry{PluginID: 951, Key: "alpha", Payload: []byte(`{}`)}
	entryC := &domain.PluginStorageEntry{PluginID: 952, Key: "gamma", Payload: []byte(`{}`)}
	require.NoError(s.T(), s.repo.Save(ctx, entryA))
	require.NoError(s.T(), s.repo.Save(ctx, entryB))
	require.NoError(s.T(), s.repo.Save(ctx, entryC))

	pluginIDsFilter := &filters.FindPluginStorage{PluginIDs: []uint64{950, 951, 952}}

	tests := []struct {
		name  string
		order []filters.Sorting
		want  func() []uint64
	}{
		{
			name: "sort_by_plugin_id_asc",
			order: []filters.Sorting{
				{Field: "plugin_id", Direction: filters.SortDirectionAsc},
			},
			want: func() []uint64 { return []uint64{entryA.ID, entryB.ID, entryC.ID} },
		},
		{
			name: "sort_by_plugin_id_desc",
			order: []filters.Sorting{
				{Field: "plugin_id", Direction: filters.SortDirectionDesc},
			},
			want: func() []uint64 { return []uint64{entryC.ID, entryB.ID, entryA.ID} },
		},
		{
			name: "sort_by_key_asc",
			order: []filters.Sorting{
				{Field: "key", Direction: filters.SortDirectionAsc},
			},
			want: func() []uint64 { return []uint64{entryB.ID, entryA.ID, entryC.ID} },
		},
		{
			name: "sort_by_key_desc",
			order: []filters.Sorting{
				{Field: "key", Direction: filters.SortDirectionDesc},
			},
			want: func() []uint64 { return []uint64{entryC.ID, entryA.ID, entryB.ID} },
		},
	}

	for _, tt := range tests {
		s.T().Run(tt.name, func(t *testing.T) {
			// ACT
			results, err := s.repo.Find(ctx, pluginIDsFilter, tt.order, nil)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, results, 3)

			gotIDs := []uint64{results[0].ID, results[1].ID, results[2].ID}
			assert.Equal(t, tt.want(), gotIDs, "entries are in wrong order")
		})
	}
}
