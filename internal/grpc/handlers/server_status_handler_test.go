package handlers

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func setupServerRepo(t *testing.T, servers ...*domain.Server) *inmemory.ServerRepository {
	t.Helper()

	repo := inmemory.NewServerRepository()
	for _, srv := range servers {
		require.NoError(t, repo.Save(context.Background(), srv))
	}

	return repo
}

func TestHandleServerStatuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		nodeID     uint64
		batch      *proto.ServerStatusBatch
		servers    []*domain.Server
		wantError  string
		assertions func(t *testing.T, repo *inmemory.ServerRepository)
	}{
		{
			name:   "nil_batch_returns_nil",
			nodeID: 1,
			batch:  nil,
		},
		{
			name:   "empty_batch_returns_nil",
			nodeID: 1,
			batch:  &proto.ServerStatusBatch{Statuses: []*proto.ServerStatus{}},
		},
		{
			name:   "single_server_status_update",
			nodeID: 1,
			batch: &proto.ServerStatusBatch{
				Statuses: []*proto.ServerStatus{
					{ServerId: 1, IsRunning: true, LastCheck: timestamppb.Now()},
				},
			},
			servers: []*domain.Server{
				{
					ID: 1, UUID: uuid.New(), UUIDShort: "s1", Name: "Server 1",
					GameID: "cs", DSID: 1, GameModID: 1, ServerIP: "127.0.0.1",
					ServerPort: 27015, Dir: "/srv/s1", ProcessActive: false,
				},
			},
			assertions: func(t *testing.T, repo *inmemory.ServerRepository) {
				t.Helper()

				servers, err := repo.Find(context.Background(), &filters.FindServer{IDs: []uint{1}}, nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 1)
				assert.True(t, servers[0].ProcessActive)
				assert.NotNil(t, servers[0].LastProcessCheck)
				assert.True(t, servers[0].LastProcessCheck.After(time.Now().Add(-10*time.Second)))
				assert.NotNil(t, servers[0].UpdatedAt)
				assert.True(t, servers[0].UpdatedAt.After(time.Now().Add(-10*time.Second)))
			},
		},
		{
			name:   "multiple_server_status_updates",
			nodeID: 1,
			batch: &proto.ServerStatusBatch{
				Statuses: []*proto.ServerStatus{
					{ServerId: 10, IsRunning: true, LastCheck: timestamppb.Now()},
					{ServerId: 11, IsRunning: false, LastCheck: timestamppb.Now()},
				},
			},
			servers: []*domain.Server{
				{
					ID: 10, UUID: uuid.New(), UUIDShort: "s10", Name: "Server 10",
					GameID: "cs", DSID: 1, GameModID: 1, ServerIP: "127.0.0.1",
					ServerPort: 27015, Dir: "/srv/s10", ProcessActive: false,
				},
				{
					ID: 11, UUID: uuid.New(), UUIDShort: "s11", Name: "Server 11",
					GameID: "cs", DSID: 1, GameModID: 1, ServerIP: "127.0.0.1",
					ServerPort: 27016, Dir: "/srv/s11", ProcessActive: true,
				},
			},
			assertions: func(t *testing.T, repo *inmemory.ServerRepository) {
				t.Helper()

				servers, err := repo.Find(context.Background(), &filters.FindServer{IDs: []uint{10, 11}}, nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 2)

				for _, srv := range servers {
					if srv.ID == 10 {
						assert.True(t, srv.ProcessActive)
					} else {
						assert.False(t, srv.ProcessActive)
					}

					assert.NotNil(t, srv.LastProcessCheck)
					assert.NotNil(t, srv.UpdatedAt)
				}
			},
		},
		{
			name:   "server_on_different_node_not_updated",
			nodeID: 1,
			batch: &proto.ServerStatusBatch{
				Statuses: []*proto.ServerStatus{
					{ServerId: 20, IsRunning: true, LastCheck: timestamppb.Now()},
				},
			},
			servers: []*domain.Server{
				{
					ID: 20, UUID: uuid.New(), UUIDShort: "s20", Name: "Server 20",
					GameID: "cs", DSID: 2, GameModID: 1, ServerIP: "127.0.0.1",
					ServerPort: 27015, Dir: "/srv/s20", ProcessActive: false,
				},
			},
			assertions: func(t *testing.T, repo *inmemory.ServerRepository) {
				t.Helper()

				servers, err := repo.Find(context.Background(), &filters.FindServer{IDs: []uint{20}}, nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 1)
				assert.False(t, servers[0].ProcessActive)
			},
		},
		{
			name:   "nonexistent_server_does_not_cause_error",
			nodeID: 1,
			batch: &proto.ServerStatusBatch{
				Statuses: []*proto.ServerStatus{
					{ServerId: 999, IsRunning: true, LastCheck: timestamppb.Now()},
				},
			},
		},
		{
			name:   "mixed_valid_and_invalid_servers",
			nodeID: 1,
			batch: &proto.ServerStatusBatch{
				Statuses: []*proto.ServerStatus{
					{ServerId: 30, IsRunning: true, LastCheck: timestamppb.Now()},
					{ServerId: 999, IsRunning: true, LastCheck: timestamppb.Now()},
				},
			},
			servers: []*domain.Server{
				{
					ID: 30, UUID: uuid.New(), UUIDShort: "s30", Name: "Server 30",
					GameID: "cs", DSID: 1, GameModID: 1, ServerIP: "127.0.0.1",
					ServerPort: 27015, Dir: "/srv/s30", ProcessActive: false,
				},
			},
			assertions: func(t *testing.T, repo *inmemory.ServerRepository) {
				t.Helper()

				servers, err := repo.Find(context.Background(), &filters.FindServer{IDs: []uint{30}}, nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 1)
				assert.True(t, servers[0].ProcessActive)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := setupServerRepo(t, tt.servers...)
			handler := NewServerStatusHandler(repo, slog.Default())

			err := handler.HandleServerStatuses(context.Background(), tt.nodeID, tt.batch)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			} else {
				require.NoError(t, err)
			}

			if tt.assertions != nil {
				tt.assertions(t, repo)
			}
		})
	}
}
