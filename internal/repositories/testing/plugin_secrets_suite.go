package testing

import (
	"context"
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type PluginSecretRepositorySuite struct {
	suite.Suite

	repo repositories.PluginSecretRepository
	fn   func(t *testing.T) repositories.PluginSecretRepository
}

func NewPluginSecretRepositorySuite(
	fn func(t *testing.T) repositories.PluginSecretRepository,
) *PluginSecretRepositorySuite {
	return &PluginSecretRepositorySuite{
		fn: fn,
	}
}

func (s *PluginSecretRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

func newPluginSecret(pluginID domain.Uint64ID, key, value string) *domain.PluginSecret {
	return &domain.PluginSecret{
		PluginID: pluginID,
		Key:      key,
		Value:    value,
	}
}

func (s *PluginSecretRepositorySuite) findByKey(
	t *testing.T,
	pluginID domain.Uint64ID,
	key string,
) []domain.PluginSecret {
	t.Helper()

	found, err := s.repo.Find(context.Background(), &filters.FindPluginSecret{
		PluginIDs: []domain.Uint64ID{pluginID},
		Keys:      []string{key},
	}, nil, nil)
	require.NoError(t, err)

	return found
}

func (s *PluginSecretRepositorySuite) TestPluginSecretRepositoryUpsert() {
	ctx := context.Background()

	s.T().Run("inserts_new_secret_and_assigns_id", func(t *testing.T) {
		entry := newPluginSecret(1, "steam_api_key", "enc:first")

		err := s.repo.Upsert(ctx, entry)
		require.NoError(t, err)
		assert.NotZero(t, entry.ID)
		require.NotNil(t, entry.CreatedAt)
		require.NotNil(t, entry.UpdatedAt)

		found := s.findByKey(t, 1, "steam_api_key")
		require.Len(t, found, 1)
		assert.Equal(t, entry.ID, found[0].ID)
		assert.Equal(t, domain.Uint64ID(1), found[0].PluginID)
		assert.Equal(t, "steam_api_key", found[0].Key)
		assert.Equal(t, "enc:first", found[0].Value)
	})

	s.T().Run("replaces_value_and_keeps_created_at", func(t *testing.T) {
		first := newPluginSecret(2, "token", "enc:first")
		require.NoError(t, s.repo.Upsert(ctx, first))
		require.NotNil(t, first.CreatedAt)

		second := newPluginSecret(2, "token", "enc:second")
		require.NoError(t, s.repo.Upsert(ctx, second))

		assert.Equal(t, first.ID, second.ID, "the upsert must reuse the row of the same (plugin_id, key)")
		require.NotNil(t, second.CreatedAt)
		assert.WithinDuration(t, *first.CreatedAt, *second.CreatedAt, 0,
			"created_at must survive the update path")

		found := s.findByKey(t, 2, "token")
		require.Len(t, found, 1)
		assert.Equal(t, "enc:second", found[0].Value)
	})

	s.T().Run("same_key_of_another_plugin_is_a_separate_row", func(t *testing.T) {
		mine := newPluginSecret(3, "shared_name", "enc:mine")
		theirs := newPluginSecret(4, "shared_name", "enc:theirs")

		require.NoError(t, s.repo.Upsert(ctx, mine))
		require.NoError(t, s.repo.Upsert(ctx, theirs))

		assert.NotEqual(t, mine.ID, theirs.ID)

		found := s.findByKey(t, 3, "shared_name")
		require.Len(t, found, 1)
		assert.Equal(t, "enc:mine", found[0].Value)
	})

	s.T().Run("keys_are_case_sensitive", func(t *testing.T) {
		lower := newPluginSecret(5, "casing", "enc:lower")
		upper := newPluginSecret(5, "Casing", "enc:upper")

		require.NoError(t, s.repo.Upsert(ctx, lower))
		require.NoError(t, s.repo.Upsert(ctx, upper))

		assert.NotEqual(t, lower.ID, upper.ID, "the unique index must be case-sensitive on every backend")

		count, err := s.repo.CountByPlugin(ctx, 5)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	s.T().Run("stores_long_ciphertext", func(t *testing.T) {
		// A TEXT column, not VARCHAR: the ciphertext of a max-size secret is
		// well past what a 255-char column would take.
		value := "enc:" + strings.Repeat("A", 12000)

		entry := newPluginSecret(6, "long", value)
		require.NoError(t, s.repo.Upsert(ctx, entry))

		found := s.findByKey(t, 6, "long")
		require.Len(t, found, 1)
		assert.Equal(t, value, found[0].Value)
	})
}

func (s *PluginSecretRepositorySuite) TestPluginSecretRepositoryFind() {
	ctx := context.Background()

	setupRepo := func(t *testing.T) {
		t.Helper()

		secrets := []*domain.PluginSecret{
			newPluginSecret(10, "alpha", "enc:a"),
			newPluginSecret(10, "beta", "enc:b"),
			newPluginSecret(11, "gamma", "enc:c"),
		}

		for _, entry := range secrets {
			require.NoError(t, s.repo.Upsert(ctx, entry))
		}
	}

	s.T().Run("filters_by_plugin", func(t *testing.T) {
		setupRepo(t)

		found, err := s.repo.Find(ctx, &filters.FindPluginSecret{
			PluginIDs: []domain.Uint64ID{10},
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, found, 2)
		assert.Equal(t, "alpha", found[0].Key)
		assert.Equal(t, "beta", found[1].Key)
	})

	s.T().Run("filters_by_plugin_and_key", func(t *testing.T) {
		setupRepo(t)

		found, err := s.repo.Find(ctx, &filters.FindPluginSecret{
			PluginIDs: []domain.Uint64ID{10},
			Keys:      []string{"beta"},
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, found, 1)
		assert.Equal(t, "enc:b", found[0].Value)
	})

	s.T().Run("key_of_another_plugin_is_not_returned", func(t *testing.T) {
		setupRepo(t)

		found, err := s.repo.Find(ctx, &filters.FindPluginSecret{
			PluginIDs: []domain.Uint64ID{10},
			Keys:      []string{"gamma"},
		}, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, found)
	})

	s.T().Run("respects_pagination_limit", func(t *testing.T) {
		setupRepo(t)

		found, err := s.repo.Find(ctx, &filters.FindPluginSecret{
			PluginIDs: []domain.Uint64ID{10},
		}, nil, &filters.Pagination{Limit: 1})
		require.NoError(t, err)
		require.Len(t, found, 1)
	})
}

func (s *PluginSecretRepositorySuite) TestPluginSecretRepositoryDelete() {
	ctx := context.Background()

	s.T().Run("deletes_single_key", func(t *testing.T) {
		require.NoError(t, s.repo.Upsert(ctx, newPluginSecret(20, "one", "enc:1")))
		require.NoError(t, s.repo.Upsert(ctx, newPluginSecret(20, "two", "enc:2")))

		require.NoError(t, s.repo.Delete(ctx, 20, "one"))

		assert.Empty(t, s.findByKey(t, 20, "one"))
		require.Len(t, s.findByKey(t, 20, "two"), 1)
	})

	s.T().Run("delete_is_scoped_to_the_plugin", func(t *testing.T) {
		require.NoError(t, s.repo.Upsert(ctx, newPluginSecret(21, "shared", "enc:mine")))
		require.NoError(t, s.repo.Upsert(ctx, newPluginSecret(22, "shared", "enc:theirs")))

		require.NoError(t, s.repo.Delete(ctx, 21, "shared"))

		assert.Empty(t, s.findByKey(t, 21, "shared"))
		require.Len(t, s.findByKey(t, 22, "shared"), 1)
	})

	s.T().Run("deleting_absent_key_is_not_an_error", func(t *testing.T) {
		require.NoError(t, s.repo.Delete(ctx, 23, "absent"))
	})
}

func (s *PluginSecretRepositorySuite) TestPluginSecretRepositoryDeleteByPlugin() {
	ctx := context.Background()

	s.T().Run("removes_every_key_of_the_plugin", func(t *testing.T) {
		for _, key := range []string{"one", "two", "three"} {
			require.NoError(t, s.repo.Upsert(ctx, newPluginSecret(30, key, "enc:"+key)))
		}
		require.NoError(t, s.repo.Upsert(ctx, newPluginSecret(31, "other", "enc:other")))

		deleted, err := s.repo.DeleteByPlugin(ctx, 30)
		require.NoError(t, err)
		assert.Equal(t, 3, deleted)

		count, err := s.repo.CountByPlugin(ctx, 30)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		remaining, err := s.repo.CountByPlugin(ctx, 31)
		require.NoError(t, err)
		assert.Equal(t, 1, remaining)
	})

	s.T().Run("reports_zero_for_plugin_without_secrets", func(t *testing.T) {
		deleted, err := s.repo.DeleteByPlugin(ctx, 32)
		require.NoError(t, err)
		assert.Equal(t, 0, deleted)
	})
}

func (s *PluginSecretRepositorySuite) TestPluginSecretRepositoryCountByPlugin() {
	ctx := context.Background()

	s.T().Run("counts_only_the_given_plugin", func(t *testing.T) {
		require.NoError(t, s.repo.Upsert(ctx, newPluginSecret(40, "one", "enc:1")))
		require.NoError(t, s.repo.Upsert(ctx, newPluginSecret(40, "two", "enc:2")))
		require.NoError(t, s.repo.Upsert(ctx, newPluginSecret(41, "other", "enc:3")))

		count, err := s.repo.CountByPlugin(ctx, 40)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	s.T().Run("overwrite_does_not_grow_the_count", func(t *testing.T) {
		require.NoError(t, s.repo.Upsert(ctx, newPluginSecret(42, "one", "enc:1")))
		require.NoError(t, s.repo.Upsert(ctx, newPluginSecret(42, "one", "enc:2")))

		count, err := s.repo.CountByPlugin(ctx, 42)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	s.T().Run("zero_for_unknown_plugin", func(t *testing.T) {
		count, err := s.repo.CountByPlugin(ctx, 43)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}
