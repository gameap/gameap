package getbusyports

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testUser = domain.User{
	ID:    1,
	Login: "testuser",
	Email: "test@example.com",
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name       string
		nodeID     string
		setupAuth  func() context.Context
		setupRepo  func(*inmemory.ServerRepository)
		wantStatus int
		wantError  string
		wantPorts  map[string][]int
	}{
		{
			name:   "successful busy ports retrieval",
			nodeID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository) {
				now := time.Now()
				queryPort1 := 27016
				rconPort1 := 27017
				queryPort2 := 27019

				server1 := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:  "short1",
					Enabled:    true,
					Name:       "Server 1",
					GameID:     "cs",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
					QueryPort:  &queryPort1,
					RconPort:   &rconPort1,
					Dir:        "/servers/server1",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				server2 := &domain.Server{
					ID:         2,
					UUID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:  "short2",
					Enabled:    true,
					Name:       "Server 2",
					GameID:     "csgo",
					DSID:       1,
					GameModID:  2,
					ServerIP:   "192.168.1.1",
					ServerPort: 27018,
					QueryPort:  &queryPort2,
					Dir:        "/servers/server2",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				server3 := &domain.Server{
					ID:         3,
					UUID:       uuid.MustParse("33333333-3333-3333-3333-333333333333"),
					UUIDShort:  "short3",
					Enabled:    true,
					Name:       "Server 3",
					GameID:     "tf2",
					DSID:       1,
					GameModID:  3,
					ServerIP:   "192.168.1.2",
					ServerPort: 27015,
					Dir:        "/servers/server3",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server1))
				require.NoError(t, serverRepo.Save(context.Background(), server2))
				require.NoError(t, serverRepo.Save(context.Background(), server3))
			},
			wantStatus: http.StatusOK,
			wantPorts: map[string][]int{
				"192.168.1.1": {27015, 27016, 27017, 27018, 27019},
				"192.168.1.2": {27015},
			},
		},
		{
			name:   "no servers on node",
			nodeID: "2",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository) {
				now := time.Now()

				server := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:  "short1",
					Enabled:    true,
					Name:       "Server 1",
					GameID:     "cs",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
					Dir:        "/servers/server1",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
			},
			wantStatus: http.StatusOK,
			wantPorts:  map[string][]int{},
		},
		{
			name:       "user not authenticated",
			nodeID:     "1",
			setupRepo:  func(_ *inmemory.ServerRepository) {},
			wantStatus: http.StatusUnauthorized,
			wantError:  "user not authenticated",
		},
		{
			name:   "invalid node id",
			nodeID: "invalid",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:  func(_ *inmemory.ServerRepository) {},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid node id",
		},
		{
			name:   "duplicate ports are deduplicated",
			nodeID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository) {
				now := time.Now()
				queryPort := 27015
				rconPort := 27015

				server := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:  "short1",
					Enabled:    true,
					Name:       "Server 1",
					GameID:     "cs",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
					QueryPort:  &queryPort,
					RconPort:   &rconPort,
					Dir:        "/servers/server1",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
			},
			wantStatus: http.StatusOK,
			wantPorts: map[string][]int{
				"192.168.1.1": {27015},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			responder := api.NewResponder()
			handler := NewHandler(serverRepo, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(serverRepo)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodGet, "/api/dedicated_servers/"+tt.nodeID+"/busy_ports", nil)
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"node": tt.nodeID})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errorMsg, tt.wantError)
			}

			if tt.wantPorts != nil {
				var ports busyPortsResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ports))
				assert.Equal(t, tt.wantPorts, map[string][]int(ports))
			}
		})
	}
}

func TestHandler_CollectBusyPorts(t *testing.T) {
	handler := &Handler{}

	now := time.Now()
	queryPort1 := 27016
	rconPort1 := 27017
	queryPort2 := 27019

	servers := []domain.Server{
		{
			ID:         1,
			UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			DSID:       1,
			ServerIP:   "192.168.1.1",
			ServerPort: 27015,
			QueryPort:  &queryPort1,
			RconPort:   &rconPort1,
			CreatedAt:  &now,
		},
		{
			ID:         2,
			UUID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			DSID:       1,
			ServerIP:   "192.168.1.1",
			ServerPort: 27018,
			QueryPort:  &queryPort2,
			CreatedAt:  &now,
		},
		{
			ID:         3,
			UUID:       uuid.MustParse("33333333-3333-3333-3333-333333333333"),
			DSID:       1,
			ServerIP:   "192.168.1.2",
			ServerPort: 27015,
			CreatedAt:  &now,
		},
	}

	result := handler.collectBusyPorts(servers)

	assert.Equal(t, 2, len(result))
	assert.Equal(t, []int{27015, 27016, 27017, 27018, 27019}, result["192.168.1.1"])
	assert.Equal(t, []int{27015}, result["192.168.1.2"])
}

func TestUniqueAndSort(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected []int
	}{
		{
			name:     "with duplicates",
			input:    []int{27015, 27016, 27015, 27017, 27016},
			expected: []int{27015, 27016, 27017},
		},
		{
			name:     "already unique and sorted",
			input:    []int{27015, 27016, 27017},
			expected: []int{27015, 27016, 27017},
		},
		{
			name:     "unsorted",
			input:    []int{27017, 27015, 27016},
			expected: []int{27015, 27016, 27017},
		},
		{
			name:     "empty",
			input:    []int{},
			expected: []int{},
		},
		{
			name:     "single element",
			input:    []int{27015},
			expected: []int{27015},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := lo.Uniq(tt.input)
			sort.Ints(result)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewHandler(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	responder := api.NewResponder()

	handler := NewHandler(serverRepo, responder)

	require.NotNil(t, handler)
	assert.Equal(t, serverRepo, handler.serversRepo)
	assert.Equal(t, responder, handler.responder)
}

func TestNewBusyPortsResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string][]int
		expected busyPortsResponse
	}{
		{
			name: "with data",
			input: map[string][]int{
				"192.168.1.1": {27015, 27016},
				"192.168.1.2": {27015},
			},
			expected: busyPortsResponse{
				"192.168.1.1": {27015, 27016},
				"192.168.1.2": {27015},
			},
		},
		{
			name:     "empty map",
			input:    map[string][]int{},
			expected: busyPortsResponse{},
		},
		{
			name:     "nil map",
			input:    nil,
			expected: busyPortsResponse{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := newBusyPortsResponse(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
