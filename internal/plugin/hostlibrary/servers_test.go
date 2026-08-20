package hostlibrary

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/common"
	"github.com/gameap/gameap/pkg/plugin/sdk/servers"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServersService_FindServers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setupRepo func(*inmemory.ServerRepository)
		request   *servers.FindServersRequest
		wantTotal int
		wantIDs   []uint
	}{
		{
			name: "no_filter_returns_all",
			setupRepo: func(r *inmemory.ServerRepository) {
				_ = r.Save(context.Background(), &domain.Server{Name: "Server1", GameID: "cs", DSID: 1})
				_ = r.Save(context.Background(), &domain.Server{Name: "Server2", GameID: "tf2", DSID: 1})
				_ = r.Save(context.Background(), &domain.Server{Name: "Server3", GameID: "mc", DSID: 2})
			},
			request:   &servers.FindServersRequest{},
			wantTotal: 3,
			wantIDs:   []uint{1, 2, 3},
		},
		{
			name: "filter_by_ids",
			setupRepo: func(r *inmemory.ServerRepository) {
				_ = r.Save(context.Background(), &domain.Server{Name: "Server1", GameID: "cs", DSID: 1})
				_ = r.Save(context.Background(), &domain.Server{Name: "Server2", GameID: "tf2", DSID: 1})
				_ = r.Save(context.Background(), &domain.Server{Name: "Server3", GameID: "mc", DSID: 2})
			},
			request: &servers.FindServersRequest{
				Filter: &servers.ServerFilter{Ids: []uint64{1, 3}},
			},
			wantTotal: 2,
			wantIDs:   []uint{1, 3},
		},
		{
			name: "filter_by_node_ids",
			setupRepo: func(r *inmemory.ServerRepository) {
				_ = r.Save(context.Background(), &domain.Server{Name: "Server1", GameID: "cs", DSID: 1})
				_ = r.Save(context.Background(), &domain.Server{Name: "Server2", GameID: "tf2", DSID: 1})
				_ = r.Save(context.Background(), &domain.Server{Name: "Server3", GameID: "mc", DSID: 2})
			},
			request: &servers.FindServersRequest{
				Filter: &servers.ServerFilter{NodeIds: []uint64{1}},
			},
			wantTotal: 2,
			wantIDs:   []uint{1, 2},
		},
		{
			name: "filter_by_game_ids",
			setupRepo: func(r *inmemory.ServerRepository) {
				_ = r.Save(context.Background(), &domain.Server{Name: "Server1", GameID: "cs", DSID: 1})
				_ = r.Save(context.Background(), &domain.Server{Name: "Server2", GameID: "cs", DSID: 1})
				_ = r.Save(context.Background(), &domain.Server{Name: "Server3", GameID: "mc", DSID: 2})
			},
			request: &servers.FindServersRequest{
				Filter: &servers.ServerFilter{GameIds: []string{"cs"}},
			},
			wantTotal: 2,
			wantIDs:   []uint{1, 2},
		},
		{
			name: "filter_by_enabled",
			setupRepo: func(r *inmemory.ServerRepository) {
				_ = r.Save(context.Background(), &domain.Server{Name: "Server1", GameID: "cs", DSID: 1, Enabled: true})
				_ = r.Save(context.Background(), &domain.Server{Name: "Server2", GameID: "tf2", DSID: 1, Enabled: false})
				_ = r.Save(context.Background(), &domain.Server{Name: "Server3", GameID: "mc", DSID: 2, Enabled: true})
			},
			request: &servers.FindServersRequest{
				Filter: &servers.ServerFilter{Enabled: new(true)},
			},
			wantTotal: 2,
			wantIDs:   []uint{1, 3},
		},
		{
			name: "pagination_applied",
			setupRepo: func(r *inmemory.ServerRepository) {
				for i := 1; i <= 10; i++ {
					_ = r.Save(context.Background(), &domain.Server{
						Name:   "Server" + string(rune('0'+i)),
						GameID: "cs",
						DSID:   1,
					})
				}
			},
			request: &servers.FindServersRequest{
				Pagination: &common.Pagination{Limit: 3, Offset: 2},
			},
			wantTotal: 3,
			wantIDs:   []uint{3, 4, 5},
		},
		{
			name:      "empty_repository_returns_empty",
			setupRepo: func(_ *inmemory.ServerRepository) {},
			request:   &servers.FindServersRequest{},
			wantTotal: 0,
			wantIDs:   []uint{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewServerRepository()
			tt.setupRepo(repo)

			svc := NewServersService(repo)
			resp, err := svc.FindServers(context.Background(), tt.request)

			require.NoError(t, err)
			assert.Equal(t, int32(tt.wantTotal), resp.Total)
			require.Len(t, resp.Servers, tt.wantTotal)

			for i, wantID := range tt.wantIDs {
				assert.Equal(t, uint64(wantID), resp.Servers[i].Id)
			}
		})
	}
}

func TestServersService_GetServer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setupRepo func(*inmemory.ServerRepository)
		serverID  uint64
		wantFound bool
		wantName  string
	}{
		{
			name: "existing_returns_found",
			setupRepo: func(r *inmemory.ServerRepository) {
				_ = r.Save(context.Background(), &domain.Server{
					Name:     "TestServer",
					GameID:   "cs",
					DSID:     1,
					ServerIP: "192.168.1.1",
				})
			},
			serverID:  1,
			wantFound: true,
			wantName:  "TestServer",
		},
		{
			name:      "missing_returns_not_found",
			setupRepo: func(_ *inmemory.ServerRepository) {},
			serverID:  999,
			wantFound: false,
		},
		{
			name: "wrong_id_returns_not_found",
			setupRepo: func(r *inmemory.ServerRepository) {
				_ = r.Save(context.Background(), &domain.Server{Name: "Server1", GameID: "cs", DSID: 1})
			},
			serverID:  999,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewServerRepository()
			tt.setupRepo(repo)

			svc := NewServersService(repo)
			resp, err := svc.GetServer(context.Background(), &servers.GetServerRequest{Id: tt.serverID})

			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, resp.Found)

			if tt.wantFound {
				require.NotNil(t, resp.Server)
				assert.Equal(t, tt.wantName, resp.Server.Name)
				assert.Equal(t, tt.serverID, resp.Server.Id)
			} else {
				assert.Nil(t, resp.Server)
			}
		})
	}
}

func TestServersService_SaveServer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		server    *proto.Server
		wantError string
	}{
		{
			name:      "nil_server_returns_error",
			server:    nil,
			wantError: "server is required",
		},
		{
			name: "valid_server_saves",
			server: &proto.Server{
				Name:       "NewServer",
				GameId:     "cs",
				DsId:       1,
				ServerIp:   "10.0.0.1",
				ServerPort: 27015,
			},
		},
		{
			name: "update_existing_server",
			server: &proto.Server{
				Id:         1,
				Name:       "UpdatedServer",
				GameId:     "cs",
				DsId:       1,
				ServerIp:   "10.0.0.1",
				ServerPort: 27015,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewServerRepository()
			if tt.server != nil && tt.server.Id > 0 {
				_ = repo.Save(context.Background(), &domain.Server{
					Name:   "OldServer",
					GameID: "cs",
					DSID:   1,
				})
			}

			svc := NewServersService(repo)
			resp, err := svc.SaveServer(context.Background(), &servers.SaveServerRequest{Server: tt.server})

			require.NoError(t, err)

			if tt.wantError != "" {
				assert.False(t, resp.Success)
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)

				return
			}

			assert.True(t, resp.Success)
			assert.Nil(t, resp.Error)
			assert.Greater(t, resp.Id, uint64(0))
		})
	}
}

func TestServersService_DeleteServer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setupRepo func(*inmemory.ServerRepository)
		serverID  uint64
	}{
		{
			name: "existing_server_deleted",
			setupRepo: func(r *inmemory.ServerRepository) {
				_ = r.Save(context.Background(), &domain.Server{Name: "ToDelete", GameID: "cs", DSID: 1})
			},
			serverID: 1,
		},
		{
			name:      "nonexistent_server_no_error",
			setupRepo: func(_ *inmemory.ServerRepository) {},
			serverID:  999,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewServerRepository()
			tt.setupRepo(repo)

			svc := NewServersService(repo)
			resp, err := svc.DeleteServer(context.Background(), &servers.DeleteServerRequest{Id: tt.serverID})

			require.NoError(t, err)
			assert.True(t, resp.Success)
		})
	}
}

func TestConvertServerToProto(t *testing.T) {
	t.Parallel()
	serverUUID := uuid.New()
	server := &domain.Server{
		ID:            42,
		UID:           serverUUID,
		Enabled:       true,
		Installed:     domain.ServerInstalledStatusInstalled,
		Blocked:       false,
		Name:          "TestServer",
		GameID:        "cs",
		DSID:          1,
		GameModID:     2,
		ServerIP:      "192.168.1.100",
		ServerPort:    27015,
		QueryPort:     new(27016),
		RconPort:      new(27017),
		Dir:           "/home/server",
		SuUser:        new("gameuser"),
		StartCommand:  new("./start.sh"),
		ProcessActive: true,
	}
	server.Hydrate()

	result := convertServerToProto(server)

	assert.Equal(t, uint64(42), result.Id)
	assert.Equal(t, serverUUID.String(), result.Uuid)
	assert.Equal(t, serverUUID.String()[:8], result.UuidShort)
	assert.True(t, result.Enabled)
	assert.Equal(t, proto.ServerInstalledStatus(domain.ServerInstalledStatusInstalled), result.Installed)
	assert.False(t, result.Blocked)
	assert.Equal(t, "TestServer", result.Name)
	assert.Equal(t, "cs", result.GameId)
	assert.Equal(t, uint64(1), result.DsId)
	assert.Equal(t, uint64(2), result.GameModId)
	assert.Equal(t, "192.168.1.100", result.ServerIp)
	assert.Equal(t, int32(27015), result.ServerPort)
	require.NotNil(t, result.QueryPort)
	assert.Equal(t, int32(27016), *result.QueryPort)
	require.NotNil(t, result.RconPort)
	assert.Equal(t, int32(27017), *result.RconPort)
	assert.Equal(t, "/home/server", result.Dir)
	require.NotNil(t, result.SuUser)
	assert.Equal(t, "gameuser", *result.SuUser)
	require.NotNil(t, result.StartCommand)
	assert.Equal(t, "./start.sh", *result.StartCommand)
	assert.True(t, result.ProcessActive)
}

func TestConvertServerToProto_NilOptionalFields(t *testing.T) {
	t.Parallel()
	server := &domain.Server{
		ID:         1,
		Name:       "BasicServer",
		GameID:     "cs",
		DSID:       1,
		ServerIP:   "127.0.0.1",
		ServerPort: 27015,
	}

	result := convertServerToProto(server)

	assert.Nil(t, result.QueryPort)
	assert.Nil(t, result.RconPort)
	assert.Nil(t, result.SuUser)
	assert.Nil(t, result.StartCommand)
}

func TestConvertServerFromProto(t *testing.T) {
	t.Parallel()
	protoServer := &proto.Server{
		Id:           42,
		Enabled:      true,
		Installed:    proto.ServerInstalledStatus_SERVER_INSTALLED_STATUS_INSTALLED,
		Blocked:      false,
		Name:         "TestServer",
		GameId:       "cs",
		DsId:         1,
		GameModId:    2,
		ServerIp:     "192.168.1.100",
		ServerPort:   27015,
		QueryPort:    new(int32(27016)),
		RconPort:     new(int32(27017)),
		Dir:          "/home/server",
		SuUser:       new("gameuser"),
		StartCommand: new("./start.sh"),
	}

	result := convertServerFromProto(protoServer)

	assert.Equal(t, uint(42), result.ID)
	assert.True(t, result.Enabled)
	assert.Equal(t, domain.ServerInstalledStatus(proto.ServerInstalledStatus_SERVER_INSTALLED_STATUS_INSTALLED), result.Installed)
	assert.False(t, result.Blocked)
	assert.Equal(t, "TestServer", result.Name)
	assert.Equal(t, "cs", result.GameID)
	assert.Equal(t, uint(1), result.DSID)
	assert.Equal(t, uint(2), result.GameModID)
	assert.Equal(t, "192.168.1.100", result.ServerIP)
	assert.Equal(t, 27015, result.ServerPort)
	require.NotNil(t, result.QueryPort)
	assert.Equal(t, 27016, *result.QueryPort)
	require.NotNil(t, result.RconPort)
	assert.Equal(t, 27017, *result.RconPort)
	assert.Equal(t, "/home/server", result.Dir)
	require.NotNil(t, result.SuUser)
	assert.Equal(t, "gameuser", *result.SuUser)
	require.NotNil(t, result.StartCommand)
	assert.Equal(t, "./start.sh", *result.StartCommand)
}

func TestNewServersHostLibrary(t *testing.T) {
	t.Parallel()
	repo := inmemory.NewServerRepository()
	lib := NewServersHostLibrary(repo)

	assert.NotNil(t, lib)
	assert.NotNil(t, lib.impl)
}
