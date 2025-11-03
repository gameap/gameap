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

type PersonalAccessTokenRepositorySuite struct {
	suite.Suite

	repo repositories.PersonalAccessTokenRepository
	fn   func(t *testing.T) repositories.PersonalAccessTokenRepository
}

func NewPersonalAccessTokenRepositorySuite(
	fn func(t *testing.T) repositories.PersonalAccessTokenRepository,
) *PersonalAccessTokenRepositorySuite {
	return &PersonalAccessTokenRepositorySuite{
		fn: fn,
	}
}

func (s *PersonalAccessTokenRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

func (s *PersonalAccessTokenRepositorySuite) TestPersonalAccessTokenRepositorySave() {
	ctx := context.Background()

	s.T().Run("insert_new_token", func(t *testing.T) {
		token := &domain.PersonalAccessToken{
			TokenableType: "user",
			TokenableID:   1,
			Name:          "Test Token",
			Token:         "test-token-123",
			Abilities: &[]domain.PATAbility{
				domain.PATAbilityServerList,
				domain.PATAbilityServerStart,
			},
		}

		err := s.repo.Save(ctx, token)
		require.NoError(t, err)
		assert.NotZero(t, token.ID)
		assert.NotNil(t, token.CreatedAt)
		assert.NotNil(t, token.UpdatedAt)
	})

	s.T().Run("update_existing_token", func(t *testing.T) {
		token := &domain.PersonalAccessToken{
			TokenableType: "user",
			TokenableID:   2,
			Name:          "Update Token",
			Token:         "update-token-456",
			Abilities:     &[]domain.PATAbility{domain.PATAbilityServerList},
		}

		err := s.repo.Save(ctx, token)
		require.NoError(t, err)
		originalID := token.ID
		originalUpdatedAt := token.UpdatedAt

		time.Sleep(10 * time.Millisecond)

		token.Name = "Updated Token Name"
		token.Abilities = &[]domain.PATAbility{
			domain.PATAbilityServerList,
			domain.PATAbilityServerStart,
		}

		err = s.repo.Save(ctx, token)
		require.NoError(t, err)
		assert.Equal(t, originalID, token.ID)
		assert.True(t, token.UpdatedAt.After(*originalUpdatedAt) || token.UpdatedAt.Equal(*originalUpdatedAt))

		filter := &filters.FindPersonalAccessToken{IDs: []uint{token.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Updated Token Name", results[0].Name)
		assert.Len(t, *results[0].Abilities, 2)
	})

	s.T().Run("save_token_with_nil_abilities", func(t *testing.T) {
		token := &domain.PersonalAccessToken{
			TokenableType: "user",
			TokenableID:   3,
			Name:          "No Abilities Token",
			Token:         "no-abilities-789",
			Abilities:     nil,
		}

		err := s.repo.Save(ctx, token)
		require.NoError(t, err)
		assert.NotZero(t, token.ID)

		filter := &filters.FindPersonalAccessToken{IDs: []uint{token.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Nil(t, results[0].Abilities)
	})

	s.T().Run("save_token_with_empty_abilities", func(t *testing.T) {
		token := &domain.PersonalAccessToken{
			TokenableType: "user",
			TokenableID:   4,
			Name:          "Empty Abilities Token",
			Token:         "empty-abilities-012",
			Abilities:     &[]domain.PATAbility{},
		}

		err := s.repo.Save(ctx, token)
		require.NoError(t, err)
		assert.NotZero(t, token.ID)
	})

	s.T().Run("auto_set_timestamps", func(t *testing.T) {
		token := &domain.PersonalAccessToken{
			TokenableType: "user",
			TokenableID:   5,
			Name:          "Timestamp Token",
			Token:         "timestamp-345",
		}

		beforeSave := time.Now()
		err := s.repo.Save(ctx, token)
		afterSave := time.Now()

		require.NoError(t, err)
		require.NotNil(t, token.CreatedAt)
		require.NotNil(t, token.UpdatedAt)
		assert.True(t, token.CreatedAt.After(beforeSave) || token.CreatedAt.Equal(beforeSave))
		assert.True(t, token.CreatedAt.Before(afterSave) || token.CreatedAt.Equal(afterSave))
	})
}

func (s *PersonalAccessTokenRepositorySuite) TestPersonalAccessTokenRepositoryFind() {
	ctx := context.Background()

	token1 := &domain.PersonalAccessToken{
		TokenableType: "user",
		TokenableID:   10,
		Name:          "User Token 1",
		Token:         "user-token-1",
		Abilities:     &[]domain.PATAbility{domain.PATAbilityServerList},
	}
	token2 := &domain.PersonalAccessToken{
		TokenableType: "user",
		TokenableID:   10,
		Name:          "User Token 2",
		Token:         "user-token-2",
		Abilities:     &[]domain.PATAbility{domain.PATAbilityServerStart},
	}
	token3 := &domain.PersonalAccessToken{
		TokenableType: "app",
		TokenableID:   20,
		Name:          "App Token",
		Token:         "app-token-1",
	}

	require.NoError(s.T(), s.repo.Save(ctx, token1))
	require.NoError(s.T(), s.repo.Save(ctx, token2))
	require.NoError(s.T(), s.repo.Save(ctx, token3))

	s.T().Run("find_by_single_id", func(t *testing.T) {
		filter := &filters.FindPersonalAccessToken{
			IDs: []uint{token1.ID},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, token1.ID, results[0].ID)
		assert.Equal(t, "User Token 1", results[0].Name)
	})

	s.T().Run("find_by_multiple_ids", func(t *testing.T) {
		filter := &filters.FindPersonalAccessToken{
			IDs: []uint{token1.ID, token3.ID},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 2)

		ids := []uint{results[0].ID, results[1].ID}
		assert.Contains(t, ids, token1.ID)
		assert.Contains(t, ids, token3.ID)
	})

	s.T().Run("find_by_token", func(t *testing.T) {
		filter := &filters.FindPersonalAccessToken{
			Tokens: []string{"user-token-2"},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "User Token 2", results[0].Name)
	})

	s.T().Run("find_by_tokenable_type", func(t *testing.T) {
		filter := &filters.FindPersonalAccessToken{
			TokenableTypes: []string{"user"},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 2)

		for _, result := range results {
			assert.Equal(t, domain.EntityType("user"), result.TokenableType)
		}
	})

	s.T().Run("find_by_tokenable_id", func(t *testing.T) {
		filter := &filters.FindPersonalAccessToken{
			TokenableIDs: []uint{10},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 2)

		for _, result := range results {
			assert.Equal(t, uint(10), result.TokenableID)
		}
	})

	s.T().Run("find_by_tokenable_type_and_id", func(t *testing.T) {
		filter := &filters.FindPersonalAccessToken{
			TokenableTypes: []string{"app"},
			TokenableIDs:   []uint{20},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "App Token", results[0].Name)
	})

	s.T().Run("find_with_nil_filter", func(t *testing.T) {
		results, err := s.repo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	s.T().Run("find_non_existent", func(t *testing.T) {
		filter := &filters.FindPersonalAccessToken{
			IDs: []uint{99999},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("find_with_pagination", func(t *testing.T) {
		filter := &filters.FindPersonalAccessToken{
			TokenableTypes: []string{"user"},
		}
		pagination := &filters.Pagination{
			Limit:  1,
			Offset: 0,
		}

		results, err := s.repo.Find(ctx, filter, nil, pagination)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	s.T().Run("find_with_order", func(t *testing.T) {
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

func (s *PersonalAccessTokenRepositorySuite) TestPersonalAccessTokenRepositoryDelete() {
	ctx := context.Background()

	s.T().Run("delete_existing_token", func(t *testing.T) {
		token := &domain.PersonalAccessToken{
			TokenableType: "user",
			TokenableID:   100,
			Name:          "Delete Token",
			Token:         "delete-token-123",
		}

		require.NoError(t, s.repo.Save(ctx, token))
		tokenID := token.ID

		err := s.repo.Delete(ctx, tokenID)
		require.NoError(t, err)

		filter := &filters.FindPersonalAccessToken{
			IDs: []uint{tokenID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("delete_non_existent_token", func(t *testing.T) {
		err := s.repo.Delete(ctx, 99999)
		require.NoError(t, err)
	})

	s.T().Run("delete_already_deleted_token", func(t *testing.T) {
		token := &domain.PersonalAccessToken{
			TokenableType: "user",
			TokenableID:   101,
			Name:          "Double Delete Token",
			Token:         "double-delete-456",
		}

		require.NoError(t, s.repo.Save(ctx, token))
		tokenID := token.ID

		err := s.repo.Delete(ctx, tokenID)
		require.NoError(t, err)

		err = s.repo.Delete(ctx, tokenID)
		require.NoError(t, err)
	})
}

func (s *PersonalAccessTokenRepositorySuite) TestPersonalAccessTokenRepositoryUpdateLastUsedAt() {
	ctx := context.Background()

	s.T().Run("update_last_used_at", func(t *testing.T) {
		token := &domain.PersonalAccessToken{
			TokenableType: "user",
			TokenableID:   200,
			Name:          "Usage Token",
			Token:         "usage-token-123",
		}

		require.NoError(t, s.repo.Save(ctx, token))
		assert.Nil(t, token.LastUsedAt)

		lastUsedTime := time.Now()
		err := s.repo.UpdateLastUsedAt(ctx, token.ID, lastUsedTime)
		require.NoError(t, err)

		filter := &filters.FindPersonalAccessToken{
			IDs: []uint{token.ID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.NotNil(t, results[0].LastUsedAt)

		t.Logf("Last used at (UTC) %v", results[0].LastUsedAt.UTC())
		t.Logf("Last used at (UTC) %v", lastUsedTime.UTC())

		assert.True(t, results[0].LastUsedAt.After(lastUsedTime.Add(-1*time.Second)))
		assert.True(t, results[0].LastUsedAt.Before(lastUsedTime.Add(1*time.Second)))
	})

	s.T().Run("update_last_used_at_multiple_times", func(t *testing.T) {
		token := &domain.PersonalAccessToken{
			TokenableType: "user",
			TokenableID:   201,
			Name:          "Multi Usage Token",
			Token:         "multi-usage-456",
		}

		require.NoError(t, s.repo.Save(ctx, token))

		firstUsage := time.Now().Add(-1 * time.Hour)
		err := s.repo.UpdateLastUsedAt(ctx, token.ID, firstUsage)
		require.NoError(t, err)

		filter := &filters.FindPersonalAccessToken{
			IDs: []uint{token.ID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.NotNil(t, results[0].LastUsedAt)

		firstLastUsedAt := *results[0].LastUsedAt

		secondUsage := time.Now()
		err = s.repo.UpdateLastUsedAt(ctx, token.ID, secondUsage)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.NotNil(t, results[0].LastUsedAt)

		assert.True(t, results[0].LastUsedAt.After(firstLastUsedAt))
	})

	s.T().Run("update_non_existent_token", func(t *testing.T) {
		err := s.repo.UpdateLastUsedAt(ctx, 99999, time.Now())
		require.NoError(t, err)
	})
}

func (s *PersonalAccessTokenRepositorySuite) TestPersonalAccessTokenRepositoryIntegration() {
	ctx := context.Background()

	s.T().Run("full_lifecycle", func(t *testing.T) {
		token := &domain.PersonalAccessToken{
			TokenableType: "user",
			TokenableID:   300,
			Name:          "Lifecycle Token",
			Token:         "lifecycle-token-123",
			Abilities: &[]domain.PATAbility{
				domain.PATAbilityServerList,
			},
		}

		err := s.repo.Save(ctx, token)
		require.NoError(t, err)
		assert.NotZero(t, token.ID)

		filter := &filters.FindPersonalAccessToken{
			IDs: []uint{token.ID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Lifecycle Token", results[0].Name)
		assert.Nil(t, results[0].LastUsedAt)

		usageTime := time.Now()
		err = s.repo.UpdateLastUsedAt(ctx, token.ID, usageTime)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.NotNil(t, results[0].LastUsedAt)

		token.Name = "Updated Lifecycle Token"
		token.Abilities = &[]domain.PATAbility{
			domain.PATAbilityServerList,
			domain.PATAbilityServerStart,
		}
		token.LastUsedAt = results[0].LastUsedAt
		err = s.repo.Save(ctx, token)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Updated Lifecycle Token", results[0].Name)
		assert.Len(t, *results[0].Abilities, 2)
		require.NotNil(t, results[0].LastUsedAt)

		err = s.repo.Delete(ctx, token.ID)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("multiple_tokens_same_user", func(t *testing.T) {
		userID := uint(400)

		token1 := &domain.PersonalAccessToken{
			TokenableType: "user",
			TokenableID:   userID,
			Name:          "User Token A",
			Token:         "user-a-token",
			Abilities:     &[]domain.PATAbility{domain.PATAbilityServerList},
		}
		token2 := &domain.PersonalAccessToken{
			TokenableType: "user",
			TokenableID:   userID,
			Name:          "User Token B",
			Token:         "user-b-token",
			Abilities:     &[]domain.PATAbility{domain.PATAbilityServerStart},
		}

		require.NoError(t, s.repo.Save(ctx, token1))
		require.NoError(t, s.repo.Save(ctx, token2))

		filter := &filters.FindPersonalAccessToken{
			TokenableTypes: []string{"user"},
			TokenableIDs:   []uint{userID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		err = s.repo.Delete(ctx, token1.ID)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, token2.ID, results[0].ID)
	})

	s.T().Run("token_with_complex_abilities", func(t *testing.T) {
		token := &domain.PersonalAccessToken{
			TokenableType: "user",
			TokenableID:   500,
			Name:          "Complex Abilities Token",
			Token:         "complex-abilities-token",
			Abilities: &[]domain.PATAbility{
				domain.PATAbilityServerList,
				domain.PATAbilityServerStart,
				domain.PATAbilityServerStop,
				domain.PATAbilityServerRestart,
				domain.PATAbilityServerUpdate,
			},
		}

		err := s.repo.Save(ctx, token)
		require.NoError(t, err)

		filter := &filters.FindPersonalAccessToken{
			IDs: []uint{token.ID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.NotNil(t, results[0].Abilities)
		assert.Len(t, *results[0].Abilities, 5)

		abilities := *results[0].Abilities
		assert.Equal(t, domain.PATAbilityServerList, abilities[0])
		assert.Equal(t, domain.PATAbilityServerStart, abilities[1])
		assert.Equal(t, domain.PATAbilityServerStop, abilities[2])
		assert.Equal(t, domain.PATAbilityServerRestart, abilities[3])
		assert.Equal(t, domain.PATAbilityServerUpdate, abilities[4])
	})
}
