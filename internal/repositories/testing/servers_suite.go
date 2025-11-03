package testing

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/google/uuid"
	"github.com/samber/lo"
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
			UUID:       uuid.New(),
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
			UUID:       uuid.New(),
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

		server.Name = "Updated Server"
		server.Installed = domain.ServerInstalledStatusInstalled
		err = s.repo.Save(ctx, server)
		require.NoError(t, err)
		assert.Equal(t, originalID, server.ID)
		assert.Equal(t, "Updated Server", server.Name)
		assert.Equal(t, domain.ServerInstalledStatusInstalled, server.Installed)
	})
}

func (s *ServerRepositorySuite) TestServerRepositoryDelete() {
	ctx := context.Background()

	s.T().Run("soft_delete_server", func(t *testing.T) {
		server := &domain.Server{
			UUID:       uuid.New(),
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
	})
}

func (s *ServerRepositorySuite) TestServerRepositoryMultipleSaves() {
	ctx := context.Background()

	servers := []domain.Server{
		{UUID: uuid.New(), UUIDShort: "srv1", Name: "Server 1", GameID: "csgo", DSID: 1, ServerIP: "127.0.0.1", ServerPort: 27015},
		{UUID: uuid.New(), UUIDShort: "srv2", Name: "Server 2", GameID: "css", DSID: 1, ServerIP: "127.0.0.1", ServerPort: 27016},
		{UUID: uuid.New(), UUIDShort: "srv3", Name: "Server 3", GameID: "tf2", DSID: 1, ServerIP: "127.0.0.1", ServerPort: 27017},
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
			UUID:       uuid.New(),
			UUIDShort:  "deleted",
			Name:       "Deleted Server",
			GameID:     "csgo",
			DSID:       1,
			ServerIP:   "127.0.0.1",
			ServerPort: 27018,
			DeletedAt:  lo.ToPtr(time.Now()),
		}
		err := s.repo.Save(ctx, deletedServer)
		require.NoError(t, err)
		assert.NotZero(t, deletedServer.ID)
	})
}

func (s *ServerRepositorySuite) TestServerRepositoryFindAll() {
	ctx := context.Background()

	server1 := &domain.Server{
		UUID:       uuid.New(),
		UUIDShort:  "findall1",
		Name:       "FindAll Server 1",
		GameID:     "csgo",
		DSID:       1,
		ServerIP:   "10.0.0.1",
		ServerPort: 27015,
		Dir:        "/servers/findall1",
	}
	server2 := &domain.Server{
		UUID:       uuid.New(),
		UUIDShort:  "findall2",
		Name:       "FindAll Server 2",
		GameID:     "minecraft",
		DSID:       2,
		ServerIP:   "10.0.0.2",
		ServerPort: 25565,
		Dir:        "/servers/findall2",
	}
	deletedServer := &domain.Server{
		UUID:       uuid.New(),
		UUIDShort:  "findall3",
		Name:       "Deleted Server",
		GameID:     "tf2",
		DSID:       3,
		ServerIP:   "10.0.0.3",
		ServerPort: 27017,
		Dir:        "/servers/findall3",
		DeletedAt:  lo.ToPtr(time.Now()),
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
		UUID:       uuid1,
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
		UUID:       uuid2,
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

func (s *ServerRepositorySuite) TestServerRepositoryFindUserServers() {
	ctx := context.Background()

	user1ID := uint(1000)
	user2ID := uint(2000)

	server1 := &domain.Server{
		UUID:       uuid.New(),
		UUIDShort:  "usersrv1",
		Name:       "User Server 1",
		GameID:     "csgo",
		DSID:       1,
		ServerIP:   "192.168.1.1",
		ServerPort: 27015,
		Dir:        "/servers/usersrv1",
	}
	server2 := &domain.Server{
		UUID:       uuid.New(),
		UUIDShort:  "usersrv2",
		Name:       "User Server 2",
		GameID:     "minecraft",
		DSID:       1,
		ServerIP:   "192.168.1.2",
		ServerPort: 25565,
		Dir:        "/servers/usersrv2",
	}
	server3 := &domain.Server{
		UUID:       uuid.New(),
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
}

func (s *ServerRepositorySuite) TestServerRepositorySaveBulk() {
	ctx := context.Background()

	s.T().Run("save_multiple_servers", func(t *testing.T) {
		servers := []*domain.Server{
			{
				UUID:       uuid.New(),
				UUIDShort:  "bulk001",
				Name:       "Bulk Server 1",
				GameID:     "csgo",
				DSID:       1,
				ServerIP:   "10.10.1.1",
				ServerPort: 27015,
				Dir:        "/servers/bulk1",
			},
			{
				UUID:       uuid.New(),
				UUIDShort:  "bulk002",
				Name:       "Bulk Server 2",
				GameID:     "minecraft",
				DSID:       1,
				ServerIP:   "10.10.1.2",
				ServerPort: 25565,
				Dir:        "/servers/bulk2",
			},
			{
				UUID:       uuid.New(),
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
			UUID:       uuid.New(),
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
		UUID:       uuid.New(),
		UUIDShort:  "setuser1",
		Name:       "SetUser Server 1",
		GameID:     "csgo",
		DSID:       1,
		ServerIP:   "192.168.2.1",
		ServerPort: 27015,
		Dir:        "/servers/setuser1",
	}
	server2 := &domain.Server{
		UUID:       uuid.New(),
		UUIDShort:  "setuser2",
		Name:       "SetUser Server 2",
		GameID:     "minecraft",
		DSID:       1,
		ServerIP:   "192.168.2.2",
		ServerPort: 25565,
		Dir:        "/servers/setuser2",
	}
	server3 := &domain.Server{
		UUID:       uuid.New(),
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
		UUID:       uuid.New(),
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

func (s *ServerRepositorySuite) TestServerRepositorySearch() {
	ctx := context.Background()

	server1 := &domain.Server{
		UUID:       uuid.New(),
		UUIDShort:  "search1",
		Name:       "CS:GO Production Server",
		GameID:     "csgo",
		DSID:       1,
		ServerIP:   "203.0.113.1",
		ServerPort: 27015,
		Dir:        "/servers/search1",
	}
	server2 := &domain.Server{
		UUID:       uuid.New(),
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
