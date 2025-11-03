package inmemory

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestServerRepository(t *testing.T) {
	suite.Run(t, repotesting.NewServerRepositorySuite(
		func(_ *testing.T) repositories.ServerRepository {
			return NewServerRepository()
		},
	))
}

func TestServerRepository_DeletedAtFiltering(t *testing.T) {
	repo := NewServerRepository()
	now := time.Now()
	deletedTime := now.Add(-1 * time.Hour)

	server1 := &domain.Server{
		ID:         1,
		UUID:       uuid.New(),
		UUIDShort:  "server1",
		Name:       "Active Server",
		GameID:     "cs",
		DSID:       1,
		ServerIP:   "127.0.0.1",
		ServerPort: 27015,
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}

	server2 := &domain.Server{
		ID:         2,
		UUID:       uuid.New(),
		UUIDShort:  "server2",
		Name:       "Deleted Server",
		GameID:     "cs",
		DSID:       1,
		ServerIP:   "127.0.0.1",
		ServerPort: 27016,
		CreatedAt:  &now,
		UpdatedAt:  &now,
		DeletedAt:  &deletedTime,
	}

	require.NoError(t, repo.Save(context.Background(), server1))
	require.NoError(t, repo.Save(context.Background(), server2))

	t.Run("FindAll_excludes_deleted_servers", func(t *testing.T) {
		servers, err := repo.FindAll(context.Background(), nil, nil)
		require.NoError(t, err)
		assert.Len(t, servers, 1)
		assert.Equal(t, "Active Server", servers[0].Name)
	})

	t.Run("Find_without_WithDeleted_excludes_deleted_servers", func(t *testing.T) {
		filter := &filters.FindServer{}
		servers, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, servers, 1)
		assert.Equal(t, "Active Server", servers[0].Name)
	})

	t.Run("Find_with_WithDeleted_includes_deleted_servers", func(t *testing.T) {
		filter := &filters.FindServer{WithDeleted: true}
		servers, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, servers, 2)
	})

	t.Run("Find_by_ID_without_WithDeleted_excludes_deleted_servers", func(t *testing.T) {
		filter := &filters.FindServer{IDs: []uint{1, 2}}
		servers, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, servers, 1)
		assert.Equal(t, uint(1), servers[0].ID)
	})

	t.Run("Find_by_ID_with_WithDeleted_includes_deleted_servers", func(t *testing.T) {
		filter := &filters.FindServer{IDs: []uint{1, 2}, WithDeleted: true}
		servers, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, servers, 2)
	})

	t.Run("FindUserServers_excludes_deleted_servers", func(t *testing.T) {
		repo.AddUserServer(1, 1)
		repo.AddUserServer(1, 2)

		servers, err := repo.FindUserServers(context.Background(), 1, nil, nil, nil)
		require.NoError(t, err)
		assert.Len(t, servers, 1)
		assert.Equal(t, "Active Server", servers[0].Name)
	})

	t.Run("FindUserServers_with_WithDeleted_includes_deleted_servers", func(t *testing.T) {
		filter := &filters.FindServer{WithDeleted: true}
		servers, err := repo.FindUserServers(context.Background(), 1, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, servers, 2)
	})
}
