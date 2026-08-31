package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ServerRepositorySuite struct {
	suite.Suite

	repo repositories.ServerRepository

	fn func(t *testing.T) repositories.ServerRepository
}

type serverRepoSetupFunc func(t *testing.T) repositories.ServerRepository

func NewServerRepositorySuite(fn serverRepoSetupFunc) *ServerRepositorySuite {
	return &ServerRepositorySuite{
		fn: fn,
	}
}

func (s *ServerRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

func (s *ServerRepositorySuite) TestServerRepositorySave() {
	ctx := context.Background()

	s.T().Run("insert_new_server", func(t *testing.T) {
		server := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "test1",
			Enabled:    true,
			Installed:  domain.ServerInstalledStatusInstalled,
			Name:       "Test Server",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "127.0.0.1",
			ServerPort: 27015,
		}

		err := s.repo.Save(ctx, server)
		require.NoError(t, err)
		assert.NotZero(t, server.ID)
		assert.NotNil(t, server.CreatedAt)
		assert.NotNil(t, server.UpdatedAt)
	})

	s.T().Run("update_existing_server", func(t *testing.T) {
		server := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "test2",
			Enabled:    true,
			Installed:  domain.ServerInstalledStatusNotInstalled,
			Name:       "Update Server",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "127.0.0.1",
			ServerPort: 27016,
		}

		err := s.repo.Save(ctx, server)
		require.NoError(t, err)
		originalID := server.ID
		originalUpdatedAt := server.UpdatedAt

		time.Sleep(10 * time.Millisecond)

		server.Name = "Updated Server"
		server.Installed = domain.ServerInstalledStatusInstalled
		err = s.repo.Save(ctx, server)
		require.NoError(t, err)
		assert.Equal(t, originalID, server.ID)
		assert.Equal(t, "Updated Server", server.Name)
		assert.Equal(t, domain.ServerInstalledStatusInstalled, server.Installed)
		assert.True(t, server.UpdatedAt.After(*originalUpdatedAt))
	})

	s.T().Run("insert_server_with_high_ports", func(t *testing.T) {
		server := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "test3",
			Enabled:    true,
			Installed:  domain.ServerInstalledStatusInstalled,
			Name:       "High Port Server",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "127.0.0.1",
			ServerPort: 47015,
			QueryPort:  new(47016),
			RconPort:   new(65535),
		}

		err := s.repo.Save(ctx, server)
		require.NoError(t, err)

		results, err := s.repo.Find(ctx, &filters.FindServer{IDs: []uint{server.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 47015, results[0].ServerPort)
		require.NotNil(t, results[0].QueryPort)
		assert.Equal(t, 47016, *results[0].QueryPort)
		require.NotNil(t, results[0].RconPort)
		assert.Equal(t, 65535, *results[0].RconPort)
	})
}

func (s *ServerRepositorySuite) TestServerRepositoryDelete() {
	ctx := context.Background()

	s.T().Run("delete_server", func(t *testing.T) {
		server := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "deltest",
			Name:       "Delete Test Server",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "127.0.0.1",
			ServerPort: 27015,
		}

		require.NoError(t, s.repo.Save(ctx, server))
		err := s.repo.Delete(ctx, server.ID)
		require.NoError(t, err)

		filter := &filters.FindServer{IDs: []uint{server.ID}, WithDeleted: true}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}

func (s *ServerRepositorySuite) TestServerRepositorySoftDelete() {
	ctx := context.Background()

	s.T().Run("soft_delete_server", func(t *testing.T) {
		server := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "softdel",
			Name:       "Soft Delete Test Server",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "127.0.0.1",
			ServerPort: 27020,
		}

		require.NoError(t, s.repo.Save(ctx, server))
		err := s.repo.SoftDelete(ctx, server.ID)
		require.NoError(t, err)

		filter := &filters.FindServer{IDs: []uint{server.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)

		filterWithDeleted := &filters.FindServer{IDs: []uint{server.ID}, WithDeleted: true}
		resultsWithDeleted, err := s.repo.Find(ctx, filterWithDeleted, nil, nil)
		require.NoError(t, err)
		require.Len(t, resultsWithDeleted, 1)
		assert.NotNil(t, resultsWithDeleted[0].DeletedAt)
	})

	s.T().Run("soft_delete_nonexistent_server", func(t *testing.T) {
		err := s.repo.SoftDelete(ctx, 99999)
		require.NoError(t, err)
	})
}

func (s *ServerRepositorySuite) TestServerRepositoryMultipleSaves() {
	ctx := context.Background()

	servers := []domain.Server{
		{UID: uuid.New(), UUIDShort: "srv1", Name: "Server 1", GameID: "csgo", DSID: 1, ServerIP: "127.0.0.1", ServerPort: 27015},
		{UID: uuid.New(), UUIDShort: "srv2", Name: "Server 2", GameID: "css", DSID: 1, ServerIP: "127.0.0.1", ServerPort: 27016},
		{UID: uuid.New(), UUIDShort: "srv3", Name: "Server 3", GameID: "tf2", DSID: 1, ServerIP: "127.0.0.1", ServerPort: 27017},
	}

	for i := range servers {
		err := s.repo.Save(ctx, &servers[i])
		require.NoError(s.T(), err)
		assert.NotZero(s.T(), servers[i].ID)
	}
}

func (s *ServerRepositorySuite) TestServerRepositoryDeletedAtHandling() {
	ctx := context.Background()

	s.T().Run("create_deleted_server", func(t *testing.T) {
		deletedServer := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "deleted",
			Name:       "Deleted Server",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "127.0.0.1",
			ServerPort: 27018,
			DeletedAt:  new(time.Now()),
		}
		err := s.repo.Save(ctx, deletedServer)
		require.NoError(t, err)
		assert.NotZero(t, deletedServer.ID)
	})
}

func (s *ServerRepositorySuite) TestServerRepositoryFindAll() {
	ctx := context.Background()

	server1 := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "findall1",
		Name:       "FindAll Server 1",
		GameID:     "csgo",
		DSID:       1,
		ServerIP:   "10.0.0.1",
		ServerPort: 27015,
		Dir:        "/servers/findall1",
	}
	server2 := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "findall2",
		Name:       "FindAll Server 2",
		GameID:     "minecraft",
		DSID:       2,
		ServerIP:   "10.0.0.2",
		ServerPort: 25565,
		Dir:        "/servers/findall2",
	}
	deletedServer := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "findall3",
		Name:       "Deleted Server",
		GameID:     "tf2",
		DSID:       3,
		ServerIP:   "10.0.0.3",
		ServerPort: 27017,
		Dir:        "/servers/findall3",
		DeletedAt:  new(time.Now()),
	}

	require.NoError(s.T(), s.repo.Save(ctx, server1))
	require.NoError(s.T(), s.repo.Save(ctx, server2))
	require.NoError(s.T(), s.repo.Save(ctx, deletedServer))

	s.T().Run("find_all_non_deleted_servers", func(t *testing.T) {
		results, err := s.repo.FindAll(ctx, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 2)

		for _, result := range results {
			assert.Nil(t, result.DeletedAt)
		}
	})

	s.T().Run("find_all_with_pagination", func(t *testing.T) {
		pagination := &filters.Pagination{Limit: 1, Offset: 0}

		results, err := s.repo.FindAll(ctx, nil, pagination)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	s.T().Run("find_all_with_order", func(t *testing.T) {
		order := []filters.Sorting{
			{Field: "id", Direction: filters.SortDirectionDesc},
		}

		results, err := s.repo.FindAll(ctx, order, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 2)

		for i := 0; i < len(results)-1; i++ {
			assert.GreaterOrEqual(t, results[i].ID, results[i+1].ID)
		}
	})
}

func (s *ServerRepositorySuite) TestServerRepositoryFind() {
	ctx := context.Background()

	uuid1 := uuid.New()
	uuid2 := uuid.New()

	server1 := &domain.Server{
		UID:        uuid1,
		UUIDShort:  "find001",
		Enabled:    true,
		Installed:  domain.ServerInstalledStatusInstalled,
		Blocked:    false,
		Name:       "Find Server 1",
		GameID:     "csgo",
		DSID:       100,
		GameModID:  1,
		ServerIP:   "172.16.0.1",
		ServerPort: 27015,
		Dir:        "/servers/find1",
	}
	server2 := &domain.Server{
		UID:        uuid2,
		UUIDShort:  "find002",
		Enabled:    false,
		Installed:  domain.ServerInstalledStatusNotInstalled,
		Blocked:    true,
		Name:       "Find Server 2",
		GameID:     "minecraft",
		DSID:       200,
		GameModID:  2,
		ServerIP:   "172.16.0.2",
		ServerPort: 25565,
		Dir:        "/servers/find2",
	}

	require.NoError(s.T(), s.repo.Save(ctx, server1))
	require.NoError(s.T(), s.repo.Save(ctx, server2))

	s.T().Run("find_by_id", func(t *testing.T) {
		filter := &filters.FindServer{IDs: []uint{server1.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, server1.ID, results[0].ID)
	})

	s.T().Run("find_by_uuid", func(t *testing.T) {
		filter := &filters.FindServer{UUIDs: []uuid.UUID{uuid1}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, uuid1, results[0].UUID)
	})

	s.T().Run("find_by_enabled", func(t *testing.T) {
		enabled := true
		filter := &filters.FindServer{Enabled: &enabled}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 1)

		for _, result := range results {
			assert.True(t, result.Enabled)
		}
	})

	s.T().Run("find_by_blocked", func(t *testing.T) {
		blocked := true
		filter := &filters.FindServer{Blocked: &blocked}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 1)

		for _, result := range results {
			assert.True(t, result.Blocked)
		}
	})

	s.T().Run("find_by_game_id", func(t *testing.T) {
		filter := &filters.FindServer{GameIDs: []string{"csgo"}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 1)

		for _, result := range results {
			assert.Equal(t, "csgo", result.GameID)
		}
	})

	s.T().Run("find_by_ds_id", func(t *testing.T) {
		filter := &filters.FindServer{DSIDs: []uint{100}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, uint(100), results[0].DSID)
	})

	s.T().Run("find_with_pagination", func(t *testing.T) {
		pagination := &filters.Pagination{Limit: 1, Offset: 0}
		results, err := s.repo.Find(ctx, nil, nil, pagination)
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

func (s *ServerRepositorySuite) TestServerRepositoryRAMLimitAboveUint32() {
	ctx := context.Background()

	s.T().Run("save_and_read_back_large_ram_limit", func(t *testing.T) {
		// ARRANGE
		// 4 GiB = uint32 max + 1 (the byte value from the bug report that overflowed
		// MySQL int unsigned); also above PostgreSQL INTEGER (~2 GiB).
		ramLimit := 4 * 1024 * 1024 * 1024
		server := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "ramlim",
			Name:       "Big RAM Server",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "127.0.0.1",
			ServerPort: 27015,
			RAMLimit:   &ramLimit,
		}

		// ACT
		require.NoError(t, s.repo.Save(ctx, server))

		// ASSERT
		results, err := s.repo.Find(ctx, &filters.FindServer{IDs: []uint{server.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.NotNil(t, results[0].RAMLimit, "ram_limit must survive the round-trip")
		assert.Equal(t, ramLimit, *results[0].RAMLimit, "ram_limit above uint32 must persist without overflow")
	})

	s.T().Run("nil_ram_limit_stays_nil", func(t *testing.T) {
		// ARRANGE
		server := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "ramnil",
			Name:       "No RAM Limit Server",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "127.0.0.1",
			ServerPort: 27016,
			RAMLimit:   nil,
		}

		// ACT
		require.NoError(t, s.repo.Save(ctx, server))

		// ASSERT
		results, err := s.repo.Find(ctx, &filters.FindServer{IDs: []uint{server.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Nil(t, results[0].RAMLimit, "nil ram_limit must remain nil after the round-trip")
	})
}

func (s *ServerRepositorySuite) TestServerRepositoryFindUserServers() {
	ctx := context.Background()

	user1ID := uint(1000)
	user2ID := uint(2000)

	server1 := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "usersrv1",
		Name:       "User Server 1",
		GameID:     "csgo",
		DSID:       1,
		ServerIP:   "192.168.1.1",
		ServerPort: 27015,
		Dir:        "/servers/usersrv1",
	}
	server2 := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "usersrv2",
		Name:       "User Server 2",
		GameID:     "minecraft",
		DSID:       1,
		ServerIP:   "192.168.1.2",
		ServerPort: 25565,
		Dir:        "/servers/usersrv2",
	}
	server3 := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "usersrv3",
		Name:       "User Server 3",
		GameID:     "tf2",
		DSID:       1,
		ServerIP:   "192.168.1.3",
		ServerPort: 27016,
		Dir:        "/servers/usersrv3",
	}

	require.NoError(s.T(), s.repo.Save(ctx, server1))
	require.NoError(s.T(), s.repo.Save(ctx, server2))
	require.NoError(s.T(), s.repo.Save(ctx, server3))

	require.NoError(s.T(), s.repo.SetUserServers(ctx, user1ID, []uint{server1.ID, server2.ID}))
	require.NoError(s.T(), s.repo.SetUserServers(ctx, user2ID, []uint{server2.ID, server3.ID}))

	s.T().Run("find_user1_servers", func(t *testing.T) {
		results, err := s.repo.FindUserServers(ctx, user1ID, nil, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		ids := []uint{results[0].ID, results[1].ID}
		assert.Contains(t, ids, server1.ID)
		assert.Contains(t, ids, server2.ID)
	})

	s.T().Run("find_user2_servers", func(t *testing.T) {
		results, err := s.repo.FindUserServers(ctx, user2ID, nil, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		ids := []uint{results[0].ID, results[1].ID}
		assert.Contains(t, ids, server2.ID)
		assert.Contains(t, ids, server3.ID)
	})

	s.T().Run("find_user_servers_with_filter", func(t *testing.T) {
		filter := &filters.FindServer{GameIDs: []string{"csgo"}}
		results, err := s.repo.FindUserServers(ctx, user1ID, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, server1.ID, results[0].ID)
	})

	s.T().Run("find_user_servers_with_pagination", func(t *testing.T) {
		pagination := &filters.Pagination{Limit: 1, Offset: 0}
		results, err := s.repo.FindUserServers(ctx, user1ID, nil, nil, pagination)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	s.T().Run("find_nonexistent_user_servers", func(t *testing.T) {
		results, err := s.repo.FindUserServers(ctx, 99999, nil, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("nil_filter_excludes_soft_deleted", func(t *testing.T) {
		require.NoError(t, s.repo.SoftDelete(ctx, server1.ID))

		results, err := s.repo.FindUserServers(ctx, user1ID, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1, "a nil filter must exclude the soft-deleted server")
		assert.Equal(t, server2.ID, results[0].ID)

		withDeleted := &filters.FindServer{WithDeleted: true}
		all, err := s.repo.FindUserServers(ctx, user1ID, withDeleted, nil, nil)
		require.NoError(t, err)
		assert.Len(t, all, 2, "WithDeleted must still return the soft-deleted server")
	})
}

func (s *ServerRepositorySuite) TestServerRepositorySaveBulk() {
	ctx := context.Background()

	s.T().Run("save_multiple_servers", func(t *testing.T) {
		servers := []*domain.Server{
			{
				UID:        uuid.New(),
				UUIDShort:  "bulk001",
				Name:       "Bulk Server 1",
				GameID:     "csgo",
				DSID:       1,
				ServerIP:   "10.10.1.1",
				ServerPort: 27015,
				Dir:        "/servers/bulk1",
			},
			{
				UID:        uuid.New(),
				UUIDShort:  "bulk002",
				Name:       "Bulk Server 2",
				GameID:     "minecraft",
				DSID:       1,
				ServerIP:   "10.10.1.2",
				ServerPort: 25565,
				Dir:        "/servers/bulk2",
			},
			{
				UID:        uuid.New(),
				UUIDShort:  "bulk003",
				Name:       "Bulk Server 3",
				GameID:     "tf2",
				DSID:       1,
				ServerIP:   "10.10.1.3",
				ServerPort: 27016,
				Dir:        "/servers/bulk3",
			},
		}

		err := s.repo.SaveBulk(ctx, servers)
		require.NoError(t, err)

		filter := &filters.FindServer{
			GameIDs: []string{"csgo", "minecraft", "tf2"},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	s.T().Run("save_bulk_empty_slice", func(t *testing.T) {
		err := s.repo.SaveBulk(ctx, []*domain.Server{})
		require.NoError(t, err)
	})

	s.T().Run("save_bulk_with_update", func(t *testing.T) {
		server := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "bulkupd",
			Name:       "Bulk Update Server",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "10.10.2.1",
			ServerPort: 27015,
			Dir:        "/servers/bulkupd",
		}

		require.NoError(t, s.repo.Save(ctx, server))
		originalID := server.ID

		server.Name = "Updated Bulk Server"
		err := s.repo.SaveBulk(ctx, []*domain.Server{server})
		require.NoError(t, err)

		filter := &filters.FindServer{IDs: []uint{originalID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Updated Bulk Server", results[0].Name)
	})
}

func (s *ServerRepositorySuite) TestServerRepositorySetUserServers() {
	ctx := context.Background()

	userID := uint(3000)

	server1 := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "setuser1",
		Name:       "SetUser Server 1",
		GameID:     "csgo",
		DSID:       1,
		ServerIP:   "192.168.2.1",
		ServerPort: 27015,
		Dir:        "/servers/setuser1",
	}
	server2 := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "setuser2",
		Name:       "SetUser Server 2",
		GameID:     "minecraft",
		DSID:       1,
		ServerIP:   "192.168.2.2",
		ServerPort: 25565,
		Dir:        "/servers/setuser2",
	}
	server3 := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "setuser3",
		Name:       "SetUser Server 3",
		GameID:     "tf2",
		DSID:       1,
		ServerIP:   "192.168.2.3",
		ServerPort: 27016,
		Dir:        "/servers/setuser3",
	}

	require.NoError(s.T(), s.repo.Save(ctx, server1))
	require.NoError(s.T(), s.repo.Save(ctx, server2))
	require.NoError(s.T(), s.repo.Save(ctx, server3))

	s.T().Run("set_user_servers_initial", func(t *testing.T) {
		err := s.repo.SetUserServers(ctx, userID, []uint{server1.ID, server2.ID})
		require.NoError(t, err)

		results, err := s.repo.FindUserServers(ctx, userID, nil, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	s.T().Run("update_user_servers", func(t *testing.T) {
		err := s.repo.SetUserServers(ctx, userID, []uint{server2.ID, server3.ID})
		require.NoError(t, err)

		results, err := s.repo.FindUserServers(ctx, userID, nil, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		ids := []uint{results[0].ID, results[1].ID}
		assert.Contains(t, ids, server2.ID)
		assert.Contains(t, ids, server3.ID)
		assert.NotContains(t, ids, server1.ID)
	})

	s.T().Run("clear_user_servers", func(t *testing.T) {
		err := s.repo.SetUserServers(ctx, userID, []uint{})
		require.NoError(t, err)

		results, err := s.repo.FindUserServers(ctx, userID, nil, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}

func (s *ServerRepositorySuite) TestServerRepositoryExists() {
	ctx := context.Background()

	server := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "exists1",
		Name:       "Exists Server",
		GameID:     "csgo",
		DSID:       1,
		ServerIP:   "192.168.3.1",
		ServerPort: 27015,
		Dir:        "/servers/exists1",
	}

	require.NoError(s.T(), s.repo.Save(ctx, server))

	s.T().Run("exists_by_id", func(t *testing.T) {
		filter := &filters.FindServer{IDs: []uint{server.ID}}
		exists, err := s.repo.Exists(ctx, filter)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	s.T().Run("exists_by_game_id", func(t *testing.T) {
		filter := &filters.FindServer{GameIDs: []string{"csgo"}}
		exists, err := s.repo.Exists(ctx, filter)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	s.T().Run("not_exists", func(t *testing.T) {
		filter := &filters.FindServer{IDs: []uint{99999}}
		exists, err := s.repo.Exists(ctx, filter)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	s.T().Run("exists_nil_filter", func(t *testing.T) {
		exists, err := s.repo.Exists(ctx, nil)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func (s *ServerRepositorySuite) TestServerRepositoryCount() {
	ctx := context.Background()

	server1 := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "count1",
		Name:       "Count Server 1",
		GameID:     "csgo",
		DSID:       10,
		Enabled:    true,
		ServerIP:   "192.168.4.1",
		ServerPort: 27015,
		Dir:        "/servers/count1",
	}
	server2 := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "count2",
		Name:       "Count Server 2",
		GameID:     "csgo",
		DSID:       11,
		Enabled:    false,
		ServerIP:   "192.168.4.2",
		ServerPort: 27016,
		Dir:        "/servers/count2",
	}
	server3 := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "count3",
		Name:       "Count Server 3",
		GameID:     "hlds",
		DSID:       10,
		Enabled:    true,
		ServerIP:   "192.168.4.3",
		ServerPort: 27017,
		Dir:        "/servers/count3",
	}
	require.NoError(s.T(), s.repo.Save(ctx, server1))
	require.NoError(s.T(), s.repo.Save(ctx, server2))
	require.NoError(s.T(), s.repo.Save(ctx, server3))

	s.T().Run("count_all_with_empty_filter", func(t *testing.T) {
		count, err := s.repo.Count(ctx, &filters.FindServer{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 3)
	})

	s.T().Run("count_by_game_id", func(t *testing.T) {
		count, err := s.repo.Count(ctx, &filters.FindServer{GameIDs: []string{"csgo"}})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 2)
	})

	s.T().Run("count_by_ds_id", func(t *testing.T) {
		count, err := s.repo.Count(ctx, &filters.FindServer{DSIDs: []uint{10}})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 2)
	})

	s.T().Run("count_by_enabled_true", func(t *testing.T) {
		enabled := true
		count, err := s.repo.Count(ctx, &filters.FindServer{Enabled: &enabled})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 2)
	})

	s.T().Run("count_no_match", func(t *testing.T) {
		count, err := s.repo.Count(ctx, &filters.FindServer{GameIDs: []string{"nonexistent_game_code"}})
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

func (s *ServerRepositorySuite) TestServerRepositorySearch() {
	ctx := context.Background()

	server1 := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "search1",
		Name:       "CS:GO Production Server",
		GameID:     "csgo",
		DSID:       1,
		ServerIP:   "203.0.113.1",
		ServerPort: 27015,
		Dir:        "/servers/search1",
	}
	server2 := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "search2",
		Name:       "Minecraft Creative Server",
		GameID:     "minecraft",
		DSID:       1,
		ServerIP:   "203.0.113.2",
		ServerPort: 25565,
		Dir:        "/servers/search2",
	}

	require.NoError(s.T(), s.repo.Save(ctx, server1))
	require.NoError(s.T(), s.repo.Save(ctx, server2))

	s.T().Run("search_by_name", func(t *testing.T) {
		results, err := s.repo.Search(ctx, "Production")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 1)
	})

	s.T().Run("search_by_ip", func(t *testing.T) {
		results, err := s.repo.Search(ctx, "203.0.113")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 2)
	})

	s.T().Run("search_with_short_query", func(t *testing.T) {
		results, err := s.repo.Search(ctx, "CS")
		require.NoError(t, err)
		assert.LessOrEqual(t, len(results), 10)
	})

	s.T().Run("search_no_results", func(t *testing.T) {
		results, err := s.repo.Search(ctx, "nonexistent")
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}

func (s *ServerRepositorySuite) TestServerRepositoryUpdateServerStatuses() {
	ctx := context.Background()

	setupRepo := func(t *testing.T) {
		t.Helper()

		servers := []*domain.Server{
			{
				ID: 100, UID: uuid.New(), UUIDShort: "st1", Enabled: true,
				Installed: domain.ServerInstalledStatusInstalled, Name: "Status Test 1",
				GameID: "cs", DSID: 5, GameModID: 1, ServerIP: "10.0.0.1",
				ServerPort: 27015, Dir: "/srv/st1", ProcessActive: false,
			},
			{
				ID: 101, UID: uuid.New(), UUIDShort: "st2", Enabled: true,
				Installed: domain.ServerInstalledStatusInstalled, Name: "Status Test 2",
				GameID: "cs", DSID: 5, GameModID: 1, ServerIP: "10.0.0.2",
				ServerPort: 27016, Dir: "/srv/st2", ProcessActive: true,
			},
			{
				ID: 102, UID: uuid.New(), UUIDShort: "st3", Enabled: true,
				Installed: domain.ServerInstalledStatusInstalled, Name: "Status Test 3",
				GameID: "cs", DSID: 6, GameModID: 1, ServerIP: "10.0.0.3",
				ServerPort: 27017, Dir: "/srv/st3", ProcessActive: false,
			},
		}

		for _, srv := range servers {
			require.NoError(t, s.repo.Save(ctx, srv))
		}
	}

	s.T().Run("update_single_server_status", func(t *testing.T) {
		setupRepo(t)

		now := time.Now()
		err := s.repo.UpdateServerStatuses(ctx, 5, []repositories.ServerStatusUpdate{
			{ID: 100, ProcessActive: true, LastProcessCheck: now},
		})
		require.NoError(t, err)

		servers, err := s.repo.Find(ctx, &filters.FindServer{IDs: []uint{100}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, servers, 1)
		assert.True(t, servers[0].ProcessActive)
		assert.NotNil(t, servers[0].LastProcessCheck)
		assert.NotNil(t, servers[0].UpdatedAt)
	})

	s.T().Run("update_multiple_server_statuses", func(t *testing.T) {
		setupRepo(t)

		now := time.Now()
		err := s.repo.UpdateServerStatuses(ctx, 5, []repositories.ServerStatusUpdate{
			{ID: 100, ProcessActive: true, LastProcessCheck: now},
			{ID: 101, ProcessActive: false, LastProcessCheck: now},
		})
		require.NoError(t, err)

		servers, err := s.repo.Find(ctx, &filters.FindServer{IDs: []uint{100, 101}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, servers, 2)

		for _, srv := range servers {
			if srv.ID == 100 {
				assert.True(t, srv.ProcessActive)
			} else {
				assert.False(t, srv.ProcessActive)
			}
		}
	})

	s.T().Run("wrong_node_id_no_change", func(t *testing.T) {
		setupRepo(t)

		now := time.Now()
		err := s.repo.UpdateServerStatuses(ctx, 5, []repositories.ServerStatusUpdate{
			{ID: 102, ProcessActive: true, LastProcessCheck: now},
		})
		require.NoError(t, err)

		servers, err := s.repo.Find(ctx, &filters.FindServer{IDs: []uint{102}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, servers, 1)
		assert.False(t, servers[0].ProcessActive)
	})

	s.T().Run("nonexistent_server_no_error", func(t *testing.T) {
		err := s.repo.UpdateServerStatuses(ctx, 5, []repositories.ServerStatusUpdate{
			{ID: 9999, ProcessActive: true, LastProcessCheck: time.Now()},
		})
		require.NoError(t, err)
	})

	s.T().Run("empty_statuses_no_error", func(t *testing.T) {
		err := s.repo.UpdateServerStatuses(ctx, 5, []repositories.ServerStatusUpdate{})
		require.NoError(t, err)
	})
}

func (s *ServerRepositorySuite) TestServerRepositoryFindFilters() {
	ctx := context.Background()

	const userID = uint(7001)

	setupRepo := func(t *testing.T) (alpha, beta, gamma *domain.Server) {
		t.Helper()

		alpha = &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "fltalph",
			Enabled:    true,
			Installed:  domain.ServerInstalledStatusInstalled,
			Blocked:    false,
			Name:       "Filter Alpha",
			GameID:     "csgo",
			DSID:       501,
			GameModID:  51,
			ServerIP:   "10.40.0.1",
			ServerPort: 27015,
			Dir:        "/servers/filter-alpha",
		}
		beta = &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "fltbeta",
			Enabled:    false,
			Installed:  domain.ServerInstalledStatusNotInstalled,
			Blocked:    true,
			Name:       "Filter Beta",
			GameID:     "minecraft",
			DSID:       502,
			GameModID:  52,
			ServerIP:   "10.40.0.2",
			ServerPort: 25565,
			Dir:        "/servers/filter-beta",
		}
		gamma = &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "fltgamma",
			Enabled:    true,
			Installed:  domain.ServerInstalledStatusInstalled,
			Blocked:    false,
			Name:       "Filter Gamma",
			GameID:     "csgo",
			DSID:       503,
			GameModID:  53,
			ServerIP:   "10.40.0.3",
			ServerPort: 28015,
			Dir:        "/servers/filter-gamma",
		}

		require.NoError(t, s.repo.Save(ctx, alpha))
		require.NoError(t, s.repo.Save(ctx, beta))
		require.NoError(t, s.repo.Save(ctx, gamma))
		require.NoError(t, s.repo.SetUserServers(ctx, userID, []uint{alpha.ID, beta.ID}))

		return alpha, beta, gamma
	}

	// Servers are created once for the whole method: subtests share one repo
	// instance, so per-subtest fixtures would accumulate and break expectations.
	alpha, beta, gamma := setupRepo(s.T())

	tests := []struct {
		name   string
		filter func(alpha, beta, gamma *domain.Server) *filters.FindServer
		want   func(alpha, beta, gamma *domain.Server) []uint
	}{
		{
			name: "find_server_by_ids",
			filter: func(alpha, _, gamma *domain.Server) *filters.FindServer {
				return filters.FindServerByIDs(alpha.ID, gamma.ID)
			},
			want: func(alpha, _, gamma *domain.Server) []uint {
				return []uint{alpha.ID, gamma.ID}
			},
		},
		{
			name: "find_server_by_ids_no_match",
			filter: func(_, _, _ *domain.Server) *filters.FindServer {
				return filters.FindServerByIDs(999991)
			},
			want: func(_, _, _ *domain.Server) []uint {
				return nil
			},
		},
		{
			name: "find_server_by_node_ids",
			filter: func(_, _, _ *domain.Server) *filters.FindServer {
				return filters.FindServerByNodeIDs(501)
			},
			want: func(alpha, _, _ *domain.Server) []uint {
				return []uint{alpha.ID}
			},
		},
		{
			name: "find_server_by_node_ids_no_match",
			filter: func(_, _, _ *domain.Server) *filters.FindServer {
				return filters.FindServerByNodeIDs(999992)
			},
			want: func(_, _, _ *domain.Server) []uint {
				return nil
			},
		},
		{
			name: "find_server_by_uuids",
			filter: func(_, beta, _ *domain.Server) *filters.FindServer {
				return filters.FindServerByUUIDs([]uuid.UUID{beta.UID})
			},
			want: func(_, beta, _ *domain.Server) []uint {
				return []uint{beta.ID}
			},
		},
		{
			name: "find_server_by_uuids_no_match",
			filter: func(_, _, _ *domain.Server) *filters.FindServer {
				return filters.FindServerByUUIDs([]uuid.UUID{uuid.New()})
			},
			want: func(_, _, _ *domain.Server) []uint {
				return nil
			},
		},
		{
			name: "find_by_user_ids",
			filter: func(_, _, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{UserIDs: []uint{userID}}
			},
			want: func(alpha, beta, _ *domain.Server) []uint {
				return []uint{alpha.ID, beta.ID}
			},
		},
		{
			name: "find_by_user_ids_no_match",
			filter: func(_, _, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{UserIDs: []uint{999993}}
			},
			want: func(_, _, _ *domain.Server) []uint {
				return nil
			},
		},
		{
			name: "find_by_enabled_false",
			filter: func(_, _, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{Enabled: new(false)}
			},
			want: func(_, beta, _ *domain.Server) []uint {
				return []uint{beta.ID}
			},
		},
		{
			name: "find_by_blocked_true",
			filter: func(_, _, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{Blocked: new(true)}
			},
			want: func(_, beta, _ *domain.Server) []uint {
				return []uint{beta.ID}
			},
		},
		{
			name: "find_by_game_mod_ids",
			filter: func(_, _, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{GameModIDs: []uint{51}}
			},
			want: func(alpha, _, _ *domain.Server) []uint {
				return []uint{alpha.ID}
			},
		},
		{
			name: "find_by_game_mod_ids_no_match",
			filter: func(_, _, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{GameModIDs: []uint{999994}}
			},
			want: func(_, _, _ *domain.Server) []uint {
				return nil
			},
		},
		{
			name: "find_by_names",
			filter: func(_, _, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{Names: []string{"Filter Gamma"}}
			},
			want: func(_, _, gamma *domain.Server) []uint {
				return []uint{gamma.ID}
			},
		},
		{
			name: "find_by_names_no_match",
			filter: func(_, _, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{Names: []string{"No Such Server"}}
			},
			want: func(_, _, _ *domain.Server) []uint {
				return nil
			},
		},
		{
			name: "find_by_ids_and_uuids",
			filter: func(alpha, beta, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{
					IDs:   []uint{alpha.ID, beta.ID},
					UUIDs: []uuid.UUID{alpha.UID},
				}
			},
			want: func(alpha, _, _ *domain.Server) []uint {
				return []uint{alpha.ID}
			},
		},
		{
			name: "find_by_ids_and_uuids_no_match",
			filter: func(alpha, beta, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{
					IDs:   []uint{alpha.ID},
					UUIDs: []uuid.UUID{beta.UID},
				}
			},
			want: func(_, _, _ *domain.Server) []uint {
				return nil
			},
		},
		{
			name: "find_by_ids_and_user_ids",
			filter: func(alpha, beta, gamma *domain.Server) *filters.FindServer {
				return &filters.FindServer{
					IDs:     []uint{alpha.ID, beta.ID, gamma.ID},
					UserIDs: []uint{userID},
				}
			},
			want: func(alpha, beta, _ *domain.Server) []uint {
				return []uint{alpha.ID, beta.ID}
			},
		},
		{
			name: "find_by_ids_and_enabled",
			filter: func(alpha, beta, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{
					IDs:     []uint{alpha.ID, beta.ID},
					Enabled: new(true),
				}
			},
			want: func(alpha, _, _ *domain.Server) []uint {
				return []uint{alpha.ID}
			},
		},
		{
			name: "find_by_ids_and_blocked",
			filter: func(alpha, beta, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{
					IDs:     []uint{alpha.ID, beta.ID},
					Blocked: new(true),
				}
			},
			want: func(_, beta, _ *domain.Server) []uint {
				return []uint{beta.ID}
			},
		},
		{
			name: "find_by_ids_and_game_ids",
			filter: func(alpha, beta, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{
					IDs:     []uint{alpha.ID, beta.ID},
					GameIDs: []string{"csgo"},
				}
			},
			want: func(alpha, _, _ *domain.Server) []uint {
				return []uint{alpha.ID}
			},
		},
		{
			name: "find_by_game_ids_and_ds_ids",
			filter: func(_, _, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{
					GameIDs: []string{"csgo"},
					DSIDs:   []uint{503},
				}
			},
			want: func(_, _, gamma *domain.Server) []uint {
				return []uint{gamma.ID}
			},
		},
		{
			name: "find_by_game_ids_and_names",
			filter: func(_, _, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{
					GameIDs: []string{"csgo"},
					Names:   []string{"Filter Gamma"},
				}
			},
			want: func(_, _, gamma *domain.Server) []uint {
				return []uint{gamma.ID}
			},
		},
		{
			name: "find_by_enabled_and_game_ids",
			filter: func(_, _, _ *domain.Server) *filters.FindServer {
				return &filters.FindServer{
					Enabled: new(true),
					GameIDs: []string{"csgo"},
				}
			},
			want: func(alpha, _, gamma *domain.Server) []uint {
				return []uint{alpha.ID, gamma.ID}
			},
		},
	}

	for _, tt := range tests {
		s.T().Run(tt.name, func(t *testing.T) {
			// ACT
			results, err := s.repo.Find(ctx, tt.filter(alpha, beta, gamma), nil, nil)

			// ASSERT
			require.NoError(t, err)

			gotIDs := make([]uint, 0, len(results))
			for i := range results {
				gotIDs = append(gotIDs, results[i].ID)
			}

			assert.ElementsMatch(t, tt.want(alpha, beta, gamma), gotIDs, "returned server IDs mismatch")
		})
	}
}

func (s *ServerRepositorySuite) TestServerRepositoryFindSorting() {
	ctx := context.Background()

	setupRepo := func(t *testing.T) (first, second, third *domain.Server) {
		t.Helper()

		createdFirst := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
		createdSecond := time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)
		createdThird := time.Date(2024, 1, 3, 10, 0, 0, 0, time.UTC)

		first = &domain.Server{
			UID:           uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			Enabled:       true,
			Installed:     domain.ServerInstalledStatusInstalled,
			Blocked:       false,
			Name:          "Alpha Sort",
			GameID:        "csgo",
			DSID:          601,
			GameModID:     61,
			ServerIP:      "10.50.0.1",
			ServerPort:    27015,
			Dir:           "/servers/sort-alpha",
			ProcessActive: true,
			CreatedAt:     &createdFirst,
		}
		second = &domain.Server{
			UID:           uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			Enabled:       false,
			Installed:     domain.ServerInstalledStatusNotInstalled,
			Blocked:       true,
			Name:          "Beta Sort",
			GameID:        "minecraft",
			DSID:          602,
			GameModID:     62,
			ServerIP:      "10.50.0.2",
			ServerPort:    25565,
			Dir:           "/servers/sort-beta",
			ProcessActive: false,
			CreatedAt:     &createdSecond,
		}
		third = &domain.Server{
			UID:           uuid.MustParse("00000000-0000-0000-0000-000000000003"),
			Enabled:       true,
			Installed:     domain.ServerInstalledStatusInstalled,
			Blocked:       false,
			Name:          "Gamma Sort",
			GameID:        "tf2",
			DSID:          603,
			GameModID:     63,
			ServerIP:      "10.50.0.3",
			ServerPort:    28015,
			Dir:           "/servers/sort-gamma",
			ProcessActive: true,
			CreatedAt:     &createdThird,
		}

		require.NoError(t, s.repo.Save(ctx, first))
		require.NoError(t, s.repo.Save(ctx, second))
		require.NoError(t, s.repo.Save(ctx, third))

		return first, second, third
	}

	// Servers are created once for the whole method: subtests share one repo
	// instance, so per-subtest fixtures would accumulate and break expectations.
	first, second, third := setupRepo(s.T())

	tests := []struct {
		name      string
		field     string
		direction filters.SortDirection
		want      func(first, second, third *domain.Server) []uint
	}{
		{
			name:      "sort_by_id_asc",
			field:     "id",
			direction: filters.SortDirectionAsc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{first.ID, second.ID, third.ID}
			},
		},
		{
			name:      "sort_by_id_desc",
			field:     "id",
			direction: filters.SortDirectionDesc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{third.ID, second.ID, first.ID}
			},
		},
		{
			name:      "sort_by_uid_asc",
			field:     "uid",
			direction: filters.SortDirectionAsc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{first.ID, second.ID, third.ID}
			},
		},
		{
			name:      "sort_by_uid_desc",
			field:     "uid",
			direction: filters.SortDirectionDesc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{third.ID, second.ID, first.ID}
			},
		},
		{
			name:      "sort_by_enabled_asc",
			field:     "enabled",
			direction: filters.SortDirectionAsc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{second.ID, first.ID, third.ID}
			},
		},
		{
			name:      "sort_by_enabled_desc",
			field:     "enabled",
			direction: filters.SortDirectionDesc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{first.ID, third.ID, second.ID}
			},
		},
		{
			name:      "sort_by_installed_asc",
			field:     "installed",
			direction: filters.SortDirectionAsc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{second.ID, first.ID, third.ID}
			},
		},
		{
			name:      "sort_by_installed_desc",
			field:     "installed",
			direction: filters.SortDirectionDesc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{first.ID, third.ID, second.ID}
			},
		},
		{
			name:      "sort_by_blocked_asc",
			field:     "blocked",
			direction: filters.SortDirectionAsc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{first.ID, third.ID, second.ID}
			},
		},
		{
			name:      "sort_by_blocked_desc",
			field:     "blocked",
			direction: filters.SortDirectionDesc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{second.ID, first.ID, third.ID}
			},
		},
		{
			name:      "sort_by_name_asc",
			field:     "name",
			direction: filters.SortDirectionAsc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{first.ID, second.ID, third.ID}
			},
		},
		{
			name:      "sort_by_name_desc",
			field:     "name",
			direction: filters.SortDirectionDesc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{third.ID, second.ID, first.ID}
			},
		},
		{
			name:      "sort_by_game_id_asc",
			field:     "game_id",
			direction: filters.SortDirectionAsc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{first.ID, second.ID, third.ID}
			},
		},
		{
			name:      "sort_by_game_id_desc",
			field:     "game_id",
			direction: filters.SortDirectionDesc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{third.ID, second.ID, first.ID}
			},
		},
		{
			name:      "sort_by_ds_id_asc",
			field:     "ds_id",
			direction: filters.SortDirectionAsc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{first.ID, second.ID, third.ID}
			},
		},
		{
			name:      "sort_by_ds_id_desc",
			field:     "ds_id",
			direction: filters.SortDirectionDesc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{third.ID, second.ID, first.ID}
			},
		},
		{
			name:      "sort_by_game_mod_id_asc",
			field:     "game_mod_id",
			direction: filters.SortDirectionAsc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{first.ID, second.ID, third.ID}
			},
		},
		{
			name:      "sort_by_game_mod_id_desc",
			field:     "game_mod_id",
			direction: filters.SortDirectionDesc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{third.ID, second.ID, first.ID}
			},
		},
		{
			name:      "sort_by_server_ip_asc",
			field:     "server_ip",
			direction: filters.SortDirectionAsc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{first.ID, second.ID, third.ID}
			},
		},
		{
			name:      "sort_by_server_ip_desc",
			field:     "server_ip",
			direction: filters.SortDirectionDesc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{third.ID, second.ID, first.ID}
			},
		},
		{
			name:      "sort_by_server_port_asc",
			field:     "server_port",
			direction: filters.SortDirectionAsc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{second.ID, first.ID, third.ID}
			},
		},
		{
			name:      "sort_by_server_port_desc",
			field:     "server_port",
			direction: filters.SortDirectionDesc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{third.ID, first.ID, second.ID}
			},
		},
		{
			name:      "sort_by_dir_asc",
			field:     "dir",
			direction: filters.SortDirectionAsc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{first.ID, second.ID, third.ID}
			},
		},
		{
			name:      "sort_by_dir_desc",
			field:     "dir",
			direction: filters.SortDirectionDesc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{third.ID, second.ID, first.ID}
			},
		},
		{
			name:      "sort_by_process_active_asc",
			field:     "process_active",
			direction: filters.SortDirectionAsc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{second.ID, first.ID, third.ID}
			},
		},
		{
			name:      "sort_by_process_active_desc",
			field:     "process_active",
			direction: filters.SortDirectionDesc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{first.ID, third.ID, second.ID}
			},
		},
		{
			name:      "sort_by_created_at_asc",
			field:     "created_at",
			direction: filters.SortDirectionAsc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{first.ID, second.ID, third.ID}
			},
		},
		{
			name:      "sort_by_created_at_desc",
			field:     "created_at",
			direction: filters.SortDirectionDesc,
			want: func(first, second, third *domain.Server) []uint {
				return []uint{third.ID, second.ID, first.ID}
			},
		},
	}

	for _, tt := range tests {
		s.T().Run(tt.name, func(t *testing.T) {
			// ARRANGE
			// Secondary id sort makes the order deterministic when the primary
			// field ties (bool fields share values between servers).
			order := []filters.Sorting{
				{Field: tt.field, Direction: tt.direction},
				{Field: "id", Direction: filters.SortDirectionAsc},
			}

			// ACT
			results, err := s.repo.Find(ctx, &filters.FindServer{}, order, nil)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, results, 3)

			gotIDs := []uint{results[0].ID, results[1].ID, results[2].ID}
			assert.Equal(t, tt.want(first, second, third), gotIDs, "servers are in wrong order")
		})
	}

	s.T().Run("sort_by_updated_at_asc", func(t *testing.T) {
		// ARRANGE
		order := []filters.Sorting{
			{Field: "updated_at", Direction: filters.SortDirectionAsc},
		}

		// ACT
		results, err := s.repo.Find(ctx, &filters.FindServer{}, order, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, results, 3)

		for i := 0; i < len(results)-1; i++ {
			require.NotNil(t, results[i].UpdatedAt)
			require.NotNil(t, results[i+1].UpdatedAt)
			assert.False(t, results[i].UpdatedAt.After(*results[i+1].UpdatedAt),
				"updated_at must be in ascending order")
		}
	})

	s.T().Run("sort_by_updated_at_desc", func(t *testing.T) {
		// ARRANGE
		order := []filters.Sorting{
			{Field: "updated_at", Direction: filters.SortDirectionDesc},
		}

		// ACT
		results, err := s.repo.Find(ctx, &filters.FindServer{}, order, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, results, 3)

		for i := 0; i < len(results)-1; i++ {
			require.NotNil(t, results[i].UpdatedAt)
			require.NotNil(t, results[i+1].UpdatedAt)
			assert.False(t, results[i].UpdatedAt.Before(*results[i+1].UpdatedAt),
				"updated_at must be in descending order")
		}
	})

	s.T().Run("find_all_with_name_order", func(t *testing.T) {
		// ARRANGE
		order := []filters.Sorting{
			{Field: "name", Direction: filters.SortDirectionDesc},
		}

		// ACT
		results, err := s.repo.FindAll(ctx, order, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, results, 3)
		assert.Equal(t, third.ID, results[0].ID, "FindAll must order by name descending")

		for i := 0; i < len(results)-1; i++ {
			assert.GreaterOrEqual(t, results[i].Name, results[i+1].Name)
		}
	})
}

// userServerLinkEditor is implemented only by repositories that expose direct
// user-server link manipulation outside the ServerRepository contract
// (currently the inmemory implementation used for fast unit tests).
type userServerLinkEditor interface {
	AddUserServer(userID uint, serverID uint)
	RemoveUserServer(userID uint, serverID uint)
}

func (s *ServerRepositorySuite) TestServerRepositoryRemoveUserServer() {
	ctx := context.Background()

	s.T().Run("remove_existing_relation", func(t *testing.T) {
		// ARRANGE
		editor, ok := s.repo.(userServerLinkEditor)
		if !ok {
			t.Skip("repository implementation does not expose AddUserServer/RemoveUserServer")
		}

		server := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "rmlink1",
			Name:       "Remove Link Server",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "192.168.5.1",
			ServerPort: 27015,
			Dir:        "/servers/rmlink1",
		}
		require.NoError(t, s.repo.Save(ctx, server))
		editor.AddUserServer(4000, server.ID)

		// ACT
		editor.RemoveUserServer(4000, server.ID)

		// ASSERT
		results, err := s.repo.FindUserServers(ctx, 4000, nil, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results, "removed relation must not show up in user servers")
	})

	s.T().Run("remove_one_of_many_relations", func(t *testing.T) {
		// ARRANGE
		editor, ok := s.repo.(userServerLinkEditor)
		if !ok {
			t.Skip("repository implementation does not expose AddUserServer/RemoveUserServer")
		}

		server1 := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "rmlink2",
			Name:       "Remove Link Server 1",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "192.168.5.2",
			ServerPort: 27015,
			Dir:        "/servers/rmlink2",
		}
		server2 := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "rmlink3",
			Name:       "Remove Link Server 2",
			GameID:     "minecraft",
			DSID:       1,
			ServerIP:   "192.168.5.3",
			ServerPort: 25565,
			Dir:        "/servers/rmlink3",
		}
		require.NoError(t, s.repo.Save(ctx, server1))
		require.NoError(t, s.repo.Save(ctx, server2))
		require.NoError(t, s.repo.SetUserServers(ctx, 4001, []uint{server1.ID, server2.ID}))

		// ACT
		editor.RemoveUserServer(4001, server1.ID)

		// ASSERT
		results, err := s.repo.FindUserServers(ctx, 4001, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, server2.ID, results[0].ID, "only the removed relation must disappear")
	})

	s.T().Run("remove_nonexistent_relation", func(t *testing.T) {
		// ARRANGE
		editor, ok := s.repo.(userServerLinkEditor)
		if !ok {
			t.Skip("repository implementation does not expose AddUserServer/RemoveUserServer")
		}

		// ACT
		editor.RemoveUserServer(99999, 99999)

		// ASSERT
		results, err := s.repo.FindUserServers(ctx, 99999, nil, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}

func (s *ServerRepositorySuite) TestServerRepositoryAttachUserServer() {
	ctx := context.Background()

	s.T().Run("attach_new_relation", func(t *testing.T) {
		// ARRANGE
		server := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "attach1",
			Name:       "Attach Link Server",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "192.168.6.1",
			ServerPort: 27015,
			Dir:        "/servers/attach1",
		}
		require.NoError(t, s.repo.Save(ctx, server))

		// ACT
		require.NoError(t, s.repo.AttachUserServer(ctx, 4100, server.ID))

		// ASSERT
		results, err := s.repo.FindUserServers(ctx, 4100, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, server.ID, results[0].ID)
	})

	s.T().Run("attach_twice_keeps_single_relation", func(t *testing.T) {
		// ARRANGE
		server := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "attach2",
			Name:       "Attach Twice Server",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "192.168.6.2",
			ServerPort: 27015,
			Dir:        "/servers/attach2",
		}
		require.NoError(t, s.repo.Save(ctx, server))
		require.NoError(t, s.repo.AttachUserServer(ctx, 4101, server.ID))

		// ACT
		require.NoError(t, s.repo.AttachUserServer(ctx, 4101, server.ID))

		// ASSERT
		results, err := s.repo.FindUserServers(ctx, 4101, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1, "repeated attach must keep a single relation")
		assert.Equal(t, server.ID, results[0].ID)
	})

	s.T().Run("attach_keeps_other_relations", func(t *testing.T) {
		// ARRANGE
		server1 := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "attach3",
			Name:       "Attach Keep Server 1",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "192.168.6.3",
			ServerPort: 27015,
			Dir:        "/servers/attach3",
		}
		server2 := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "attach4",
			Name:       "Attach Keep Server 2",
			GameID:     "minecraft",
			DSID:       1,
			ServerIP:   "192.168.6.4",
			ServerPort: 25565,
			Dir:        "/servers/attach4",
		}
		require.NoError(t, s.repo.Save(ctx, server1))
		require.NoError(t, s.repo.Save(ctx, server2))
		require.NoError(t, s.repo.AttachUserServer(ctx, 4102, server1.ID))

		// ACT
		require.NoError(t, s.repo.AttachUserServer(ctx, 4102, server2.ID))

		// ASSERT
		results, err := s.repo.FindUserServers(ctx, 4102, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.ElementsMatch(t, []uint{server1.ID, server2.ID},
			[]uint{results[0].ID, results[1].ID},
			"both the old and the new relation must be present")
	})
}

func (s *ServerRepositorySuite) TestServerRepositoryAttachUserServerConcurrent() {
	ctx := context.Background()

	server := &domain.Server{
		UID:        uuid.New(),
		UUIDShort:  "attachc",
		Name:       "Attach Concurrent Server",
		GameID:     "csgo",
		DSID:       1,
		ServerIP:   "192.168.6.5",
		ServerPort: 27015,
		Dir:        "/servers/attachc",
	}
	require.NoError(s.T(), s.repo.Save(ctx, server))

	var eg errgroup.Group
	for range 8 {
		eg.Go(func() error {
			return s.repo.AttachUserServer(ctx, 4120, server.ID)
		})
	}
	require.NoError(s.T(), eg.Wait())

	results, err := s.repo.FindUserServers(ctx, 4120, nil, nil, nil)
	require.NoError(s.T(), err)
	require.Len(s.T(), results, 1,
		"concurrent attaches of the same pair must keep a single relation")
	assert.Equal(s.T(), server.ID, results[0].ID)
}

func (s *ServerRepositorySuite) TestServerRepositoryDetachUserServer() {
	ctx := context.Background()

	s.T().Run("detach_existing_relation", func(t *testing.T) {
		// ARRANGE
		server := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "detach1",
			Name:       "Detach Link Server",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "192.168.7.1",
			ServerPort: 27015,
			Dir:        "/servers/detach1",
		}
		require.NoError(t, s.repo.Save(ctx, server))
		require.NoError(t, s.repo.AttachUserServer(ctx, 4110, server.ID))

		// ACT
		require.NoError(t, s.repo.DetachUserServer(ctx, 4110, server.ID))

		// ASSERT
		results, err := s.repo.FindUserServers(ctx, 4110, nil, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("detach_only_requested_pair", func(t *testing.T) {
		// ARRANGE
		server1 := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "detach2",
			Name:       "Detach Pair Server 1",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "192.168.7.2",
			ServerPort: 27015,
			Dir:        "/servers/detach2",
		}
		server2 := &domain.Server{
			UID:        uuid.New(),
			UUIDShort:  "detach3",
			Name:       "Detach Pair Server 2",
			GameID:     "minecraft",
			DSID:       1,
			ServerIP:   "192.168.7.3",
			ServerPort: 25565,
			Dir:        "/servers/detach3",
		}
		require.NoError(t, s.repo.Save(ctx, server1))
		require.NoError(t, s.repo.Save(ctx, server2))
		require.NoError(t, s.repo.AttachUserServer(ctx, 4111, server1.ID))
		require.NoError(t, s.repo.AttachUserServer(ctx, 4111, server2.ID))
		require.NoError(t, s.repo.AttachUserServer(ctx, 4112, server1.ID))

		// ACT
		require.NoError(t, s.repo.DetachUserServer(ctx, 4111, server1.ID))

		// ASSERT
		results, err := s.repo.FindUserServers(ctx, 4111, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, server2.ID, results[0].ID, "only the detached pair must disappear")

		otherResults, err := s.repo.FindUserServers(ctx, 4112, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, otherResults, 1, "other users' relations to the same server must survive")
		assert.Equal(t, server1.ID, otherResults[0].ID)
	})

	s.T().Run("detach_nonexistent_relation_is_noop", func(t *testing.T) {
		// ACT
		err := s.repo.DetachUserServer(ctx, 4113, 99999)

		// ASSERT
		require.NoError(t, err)

		results, err := s.repo.FindUserServers(ctx, 4113, nil, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}

func (s *ServerRepositorySuite) TestServerRepositoryPagination() {
	ctx := context.Background()

	setupRepo := func(t *testing.T) []uint {
		t.Helper()

		ids := make([]uint, 0, 5)
		for i := 1; i <= 5; i++ {
			server := &domain.Server{
				UID:        uuid.New(),
				UUIDShort:  fmt.Sprintf("page%d", i),
				Name:       fmt.Sprintf("Pagination Server %d", i),
				GameID:     "csgo",
				DSID:       1,
				ServerIP:   "10.60.0.1",
				ServerPort: 27014 + i,
				Dir:        fmt.Sprintf("/servers/page%d", i),
			}
			require.NoError(t, s.repo.Save(ctx, server))
			ids = append(ids, server.ID)
		}

		return ids
	}

	tests := []struct {
		name       string
		pagination *filters.Pagination
		want       func(ids []uint) []uint
	}{
		{
			name:       "limit_only",
			pagination: &filters.Pagination{Limit: 2, Offset: 0},
			want: func(ids []uint) []uint {
				return ids[0:2]
			},
		},
		{
			name:       "limit_and_offset",
			pagination: &filters.Pagination{Limit: 2, Offset: 2},
			want: func(ids []uint) []uint {
				return ids[2:4]
			},
		},
		{
			name:       "offset_on_last_page",
			pagination: &filters.Pagination{Limit: 2, Offset: 4},
			want: func(ids []uint) []uint {
				return ids[4:5]
			},
		},
		{
			name:       "offset_beyond_total",
			pagination: &filters.Pagination{Limit: 2, Offset: 100},
			want: func(_ []uint) []uint {
				return nil
			},
		},
		{
			name:       "offset_exactly_at_end",
			pagination: &filters.Pagination{Limit: 2, Offset: 5},
			want: func(_ []uint) []uint {
				return nil
			},
		},
		{
			name:       "zero_limit_applies_default",
			pagination: &filters.Pagination{Limit: 0, Offset: 0},
			want: func(ids []uint) []uint {
				return ids
			},
		},
		{
			name:       "limit_larger_than_total",
			pagination: &filters.Pagination{Limit: 100, Offset: 0},
			want: func(ids []uint) []uint {
				return ids
			},
		},
	}

	// Servers are created once for the whole method: subtests share one repo
	// instance, so per-subtest fixtures would accumulate and break expectations.
	ids := setupRepo(s.T())

	for _, tt := range tests {
		s.T().Run(tt.name, func(t *testing.T) {
			// ACT
			results, err := s.repo.Find(ctx, nil, nil, tt.pagination)

			// ASSERT
			require.NoError(t, err)

			gotIDs := make([]uint, 0, len(results))
			for i := range results {
				gotIDs = append(gotIDs, results[i].ID)
			}

			wantIDs := tt.want(ids)
			if len(wantIDs) == 0 {
				assert.Empty(t, gotIDs, "paginated server IDs must be empty")
			} else {
				assert.Equal(t, wantIDs, gotIDs, "paginated server IDs mismatch")
			}
		})
	}

	s.T().Run("find_all_limit_and_offset", func(t *testing.T) {
		// ARRANGE
		pagination := &filters.Pagination{Limit: 2, Offset: 1}

		// ACT
		results, err := s.repo.FindAll(ctx, nil, pagination)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Equal(t, ids[1], results[0].ID)
		assert.Equal(t, ids[2], results[1].ID)
	})

	s.T().Run("find_all_offset_beyond_total", func(t *testing.T) {
		// ARRANGE
		pagination := &filters.Pagination{Limit: 2, Offset: 100}

		// ACT
		results, err := s.repo.FindAll(ctx, nil, pagination)

		// ASSERT
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}
