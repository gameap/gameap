package paste

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:unparam
func allowUserFilesAbility(t *testing.T, rbacRepo *inmemory.RBACRepository, userID, serverID uint) {
	t.Helper()

	ability := &domain.Ability{
		Name:       domain.AbilityNameGameServerFiles,
		EntityType: lo.ToPtr(domain.EntityTypeServer),
		EntityID:   new(serverID),
	}
	require.NoError(t, rbacRepo.SaveAbility(context.Background(), ability))

	permission := &domain.Permission{
		AbilityID:  ability.ID,
		EntityID:   new(userID),
		EntityType: lo.ToPtr(domain.EntityTypeUser),
		Forbidden:  false,
	}
	require.NoError(t, rbacRepo.SavePermission(context.Background(), permission))
}

var testUser1 = domain.User{
	ID:    1,
	Login: "testuser",
	Email: "test@example.com",
}

var testUser2 = domain.User{
	ID:    2,
	Login: "admin",
	Email: "admin@example.com",
}

var testNode = domain.Node{
	ID:                  1,
	Enabled:             true,
	Name:                "Test Node",
	OS:                  "linux",
	Location:            "Test Location",
	GdaemonHost:         "127.0.0.1",
	GdaemonPort:         31717,
	GdaemonAPIKey:       "test-key",
	WorkPath:            "/srv/gameap",
	GdaemonServerCert:   "test-cert",
	ClientCertificateID: 1,
}

type mockFileService struct {
	copyFunc  func(ctx context.Context, node *domain.Node, source, destination string) error
	moveFunc  func(ctx context.Context, node *domain.Node, source, destination string) error
	copyCalls int
	moveCalls int
}

func (m *mockFileService) Copy(
	ctx context.Context,
	node *domain.Node,
	source string,
	destination string,
) error {
	m.copyCalls++

	if m.copyFunc != nil {
		return m.copyFunc(ctx, node, source, destination)
	}

	return nil
}

func (m *mockFileService) Move(
	ctx context.Context,
	node *domain.Node,
	source string,
	destination string,
) error {
	m.moveCalls++

	if m.moveFunc != nil {
		return m.moveFunc(ctx, node, source, destination)
	}

	return nil
}

func testUser1Session() context.Context {
	session := &auth.Session{
		Login: "testuser",
		Email: "test@example.com",
		User:  &testUser1,
	}

	return auth.ContextWithSession(context.Background(), session)
}

func setupServer1WithNode(
	t *testing.T,
	serverRepo *inmemory.ServerRepository,
	nodeRepo *inmemory.NodeRepository,
	rbacRepo *inmemory.RBACRepository,
) {
	t.Helper()

	now := time.Now()

	server := &domain.Server{
		ID:            1,
		UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		UUIDShort:     "short1",
		Enabled:       true,
		Installed:     1,
		Blocked:       false,
		Name:          "Test Server 1",
		GameID:        "cs",
		DSID:          1,
		GameModID:     1,
		ServerIP:      "127.0.0.1",
		ServerPort:    27015,
		Dir:           "servers/test1",
		ProcessActive: false,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}

	require.NoError(t, serverRepo.Save(context.Background(), server))
	serverRepo.AddUserServer(1, 1)
	allowUserFilesAbility(t, rbacRepo, 1, 1)

	node := testNode
	require.NoError(t, nodeRepo.Save(context.Background(), &node))
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name             string
		serverID         string
		requestBody      any
		setupAuth        func() context.Context
		setupRepo        func(*inmemory.ServerRepository, *inmemory.NodeRepository, *inmemory.RBACRepository)
		setupFileService func() *mockFileService
		expectedStatus   int
		wantError        string
		wantCopyCalls    *int
		wantMoveCalls    *int
		validateResponse func(*testing.T, []byte)
	}{
		{
			name:     "successful_copy_single_file",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"test.txt"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						assert.Equal(t, "/srv/gameap/servers/test1/test.txt", source)
						assert.Equal(t, "/srv/gameap/servers/test1/new/test.txt", destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response pasteResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Equal(t, "Copied successfully!", response.Result.Message)
			},
		},
		{
			name:     "successful_copy_windows_backslash_path",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"ioquake3\\SDL264.dll"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						assert.Equal(t, "/srv/gameap/servers/test1/ioquake3/SDL264.dll", source)
						assert.Equal(t, "/srv/gameap/servers/test1/new/SDL264.dll", destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response pasteResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Equal(t, "Copied successfully!", response.Result.Message)
			},
		},
		{
			name:     "successful_cut_single_file",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "some-dir",
				Clipboard: clipboard{
					Type:        "cut",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"video.mp4"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					moveFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						assert.Equal(t, "/srv/gameap/servers/test1/video.mp4", source)
						assert.Equal(t, "/srv/gameap/servers/test1/some-dir/video.mp4", destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response pasteResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Equal(t, "Moved successfully!", response.Result.Message)
			},
		},
		{
			name:     "successful_copy_multiple_files_and_directories",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "backup",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{"configs", "logs"},
					Files:       []string{"server.cfg", "autoexec.cfg"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				callCount := 0
				expectedCalls := []struct {
					source      string
					destination string
				}{
					{
						source:      "/srv/gameap/servers/test1/server.cfg",
						destination: "/srv/gameap/servers/test1/backup/server.cfg",
					},
					{
						source:      "/srv/gameap/servers/test1/autoexec.cfg",
						destination: "/srv/gameap/servers/test1/backup/autoexec.cfg",
					},
					{
						source:      "/srv/gameap/servers/test1/configs",
						destination: "/srv/gameap/servers/test1/backup/configs",
					},
					{
						source:      "/srv/gameap/servers/test1/logs",
						destination: "/srv/gameap/servers/test1/backup/logs",
					},
				}

				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						require.Less(t, callCount, len(expectedCalls), "more calls than expected")
						assert.Equal(t, expectedCalls[callCount].source, source)
						assert.Equal(t, expectedCalls[callCount].destination, destination)
						callCount++

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response pasteResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Equal(t, "Copied successfully!", response.Result.Message)
			},
		},
		{
			name:     "successful_paste_to_root_directory",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: ".",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"logs/latest.log"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						assert.Equal(t, "/srv/gameap/servers/test1/logs/latest.log", source)
						assert.Equal(t, "/srv/gameap/servers/test1/latest.log", destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response pasteResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
			},
		},
		{
			name:     "unsupported_disk",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "local",
				Path: "new",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"test.txt"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "unsupported disk",
		},
		{
			name:     "unsupported_clipboard_disk",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "local",
					Directories: []string{},
					Files:       []string{"test.txt"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "unsupported clipboard disk",
		},
		{
			name:     "invalid_clipboard_type",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "invalid",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"test.txt"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "unsupported clipboard type",
		},
		{
			name:     "empty_clipboard",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "clipboard is empty",
		},
		{
			name:     "user_not_authenticated",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"test.txt"},
				},
			},
			//nolint:gocritic
			setupAuth: func() context.Context {
				return context.Background()
			},
			setupRepo: func(_ *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.RBACRepository) {
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:     "invalid_server_id",
			serverID: "invalid",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"test.txt"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(_ *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.RBACRepository) {
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
		},
		{
			name:     "server_not_found",
			serverID: "999",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"test.txt"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(_ *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.RBACRepository) {
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
		},
		{
			name:     "user_does_not_have_access_to_server",
			serverID: "2",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"test.txt"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            2,
					UID:           uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:     "short2",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Other User Server",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27016,
					Dir:           "servers/test2",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(2, 2)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
		},
		{
			name:     "admin_can_paste_to_any_server",
			serverID: "2",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"test.txt"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser2,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            2,
					UID:           uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:     "short2",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Server 2",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27016,
					Dir:           "servers/test2",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 2)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))

				adminAbility := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser2.ID, adminAbility.ID))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, _ string, _ string) error {
						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response pasteResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
			},
		},
		{
			name:     "invalid_path_with_directory_traversal",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "../../../etc",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"test.txt"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "path contains invalid directory traversal",
		},
		{
			name:     "invalid_source_file_path_with_directory_traversal",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"../../../etc/passwd"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "path contains invalid directory traversal",
		},
		{
			name:     "invalid_source_directory_path_with_directory_traversal",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{"../../../etc"},
					Files:       []string{},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "path contains invalid directory traversal",
		},
		{
			name:     "node_not_found",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"test.txt"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          999,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "node not found",
		},
		{
			name:        "invalid_request_body",
			serverID:    "1",
			requestBody: "invalid json",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid request body",
		},
		{
			name:     "user_without_files_permission",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"test.txt"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.NodeRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusForbidden,
			wantError:      "user does not have required permissions",
		},
		{
			name:     "admin_bypasses_files_permission",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"test.txt"},
				},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser2,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))

				adminAbility := &domain.Ability{
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser2.ID, adminAbility.ID))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, _ string, _ string) error {
						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response pasteResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
			},
		},
		{
			name:     "copy_to_same_directory_without_rename",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "docs",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"docs/report.txt"},
				},
			},
			setupAuth: testUser1Session,
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				setupServer1WithNode(t, serverRepo, nodeRepo, rbacRepo)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						t.Errorf("Copy must not be called, got %s -> %s", source, destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "copy source and destination are the same",
		},
		{
			name:     "copy_to_same_directory_with_rename_suffix",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "docs",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"docs/report.txt"},
				},
				Names: map[string]string{"docs/report.txt": "report_1.txt"},
			},
			setupAuth: testUser1Session,
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				setupServer1WithNode(t, serverRepo, nodeRepo, rbacRepo)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						assert.Equal(t, "/srv/gameap/servers/test1/docs/report.txt", source)
						assert.Equal(t, "/srv/gameap/servers/test1/docs/report_1.txt", destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			wantCopyCalls:  new(1),
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response pasteResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Equal(t, "Copied successfully!", response.Result.Message)
			},
		},
		{
			name:     "copy_windows_backslash_path_to_same_directory_with_rename_suffix",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "ioquake3",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"ioquake3\\SDL264.dll"},
				},
				Names: map[string]string{"ioquake3\\SDL264.dll": "SDL264_1.dll"},
			},
			setupAuth: testUser1Session,
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				setupServer1WithNode(t, serverRepo, nodeRepo, rbacRepo)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						assert.Equal(t, "/srv/gameap/servers/test1/ioquake3/SDL264.dll", source)
						assert.Equal(t, "/srv/gameap/servers/test1/ioquake3/SDL264_1.dll", destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			wantCopyCalls:  new(1),
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response pasteResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
			},
		},
		{
			name:     "copy_directory_to_same_directory_with_rename_suffix",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: ".",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{"configs"},
					Files:       []string{},
				},
				Names: map[string]string{"configs": "configs_1"},
			},
			setupAuth: testUser1Session,
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				setupServer1WithNode(t, serverRepo, nodeRepo, rbacRepo)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						assert.Equal(t, "/srv/gameap/servers/test1/configs", source)
						assert.Equal(t, "/srv/gameap/servers/test1/configs_1", destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			wantCopyCalls:  new(1),
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response pasteResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
			},
		},
		{
			name:     "cut_to_same_directory_is_noop",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "docs",
				Clipboard: clipboard{
					Type:        "cut",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"docs/report.txt"},
				},
			},
			setupAuth: testUser1Session,
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				setupServer1WithNode(t, serverRepo, nodeRepo, rbacRepo)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					moveFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						t.Errorf("Move must not be called, got %s -> %s", source, destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			wantMoveCalls:  new(0),
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response pasteResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Equal(t, "Moved successfully!", response.Result.Message)
			},
		},
		{
			name:     "cut_directory_to_same_directory_is_noop",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: ".",
				Clipboard: clipboard{
					Type:        "cut",
					Disk:        "server",
					Directories: []string{"configs"},
					Files:       []string{},
				},
			},
			setupAuth: testUser1Session,
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				setupServer1WithNode(t, serverRepo, nodeRepo, rbacRepo)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					moveFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						t.Errorf("Move must not be called, got %s -> %s", source, destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			wantMoveCalls:  new(0),
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response pasteResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Equal(t, "Moved successfully!", response.Result.Message)
			},
		},
		{
			name:     "cut_mixed_items_moves_only_relocated_ones",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "new",
				Clipboard: clipboard{
					Type:        "cut",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"new/a.txt", "b.txt"},
				},
			},
			setupAuth: testUser1Session,
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				setupServer1WithNode(t, serverRepo, nodeRepo, rbacRepo)
			},
			setupFileService: func() *mockFileService {
				callCount := 0

				return &mockFileService{
					moveFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						require.Equal(t, 0, callCount, "only one move expected")
						assert.Equal(t, "/srv/gameap/servers/test1/b.txt", source)
						assert.Equal(t, "/srv/gameap/servers/test1/new/b.txt", destination)
						callCount++

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			wantMoveCalls:  new(1),
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response pasteResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Equal(t, "Moved successfully!", response.Result.Message)
			},
		},
		{
			name:     "invalid_name_with_path_separator",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "backup",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"docs/report.txt"},
				},
				Names: map[string]string{"docs/report.txt": "sub/report.txt"},
			},
			setupAuth: testUser1Session,
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				setupServer1WithNode(t, serverRepo, nodeRepo, rbacRepo)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						t.Errorf("Copy must not be called, got %s -> %s", source, destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "filename contains path separators",
		},
		{
			name:     "invalid_name_with_directory_traversal",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "backup",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"docs/report.txt"},
				},
				Names: map[string]string{"docs/report.txt": ".."},
			},
			setupAuth: testUser1Session,
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				setupServer1WithNode(t, serverRepo, nodeRepo, rbacRepo)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						t.Errorf("Copy must not be called, got %s -> %s", source, destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "filename contains invalid directory traversal",
		},
		{
			name:     "invalid_empty_name",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "backup",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"docs/report.txt"},
				},
				Names: map[string]string{"docs/report.txt": ""},
			},
			setupAuth: testUser1Session,
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				setupServer1WithNode(t, serverRepo, nodeRepo, rbacRepo)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						t.Errorf("Copy must not be called, got %s -> %s", source, destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "filename is empty",
		},
		{
			name:     "directory_pasted_into_itself",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "configs",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{"configs"},
					Files:       []string{},
				},
			},
			setupAuth: testUser1Session,
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				setupServer1WithNode(t, serverRepo, nodeRepo, rbacRepo)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						t.Errorf("Copy must not be called, got %s -> %s", source, destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "into itself",
		},
		{
			name:     "directory_pasted_into_own_subdirectory",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "configs/nested",
				Clipboard: clipboard{
					Type:        "cut",
					Disk:        "server",
					Directories: []string{"configs"},
					Files:       []string{},
				},
			},
			setupAuth: testUser1Session,
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				setupServer1WithNode(t, serverRepo, nodeRepo, rbacRepo)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					moveFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						t.Errorf("Move must not be called, got %s -> %s", source, destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "into itself",
		},
		{
			name:     "directory_pasted_into_itself_with_rename",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "configs",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{"configs"},
					Files:       []string{},
				},
				Names: map[string]string{"configs": "configs_1"},
			},
			setupAuth: testUser1Session,
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				setupServer1WithNode(t, serverRepo, nodeRepo, rbacRepo)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						t.Errorf("Copy must not be called, got %s -> %s", source, destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "into itself",
		},
		{
			name:     "names_applied_only_to_listed_items",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "backup",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"a.txt", "docs/b.txt"},
				},
				Names: map[string]string{"docs/b.txt": "b_1.txt"},
			},
			setupAuth: testUser1Session,
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				setupServer1WithNode(t, serverRepo, nodeRepo, rbacRepo)
			},
			setupFileService: func() *mockFileService {
				callCount := 0
				expectedCalls := []struct {
					source      string
					destination string
				}{
					{
						source:      "/srv/gameap/servers/test1/a.txt",
						destination: "/srv/gameap/servers/test1/backup/a.txt",
					},
					{
						source:      "/srv/gameap/servers/test1/docs/b.txt",
						destination: "/srv/gameap/servers/test1/backup/b_1.txt",
					},
				}

				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, source string, destination string) error {
						require.Less(t, callCount, len(expectedCalls), "more calls than expected")
						assert.Equal(t, expectedCalls[callCount].source, source)
						assert.Equal(t, expectedCalls[callCount].destination, destination)
						callCount++

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			wantCopyCalls:  new(2),
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response pasteResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Equal(t, "Copied successfully!", response.Result.Message)
			},
		},
		{
			name:     "invalid_item_aborts_batch_before_any_operation",
			serverID: "1",
			requestBody: pasteRequest{
				Disk: "server",
				Path: "backup",
				Clipboard: clipboard{
					Type:        "copy",
					Disk:        "server",
					Directories: []string{},
					Files:       []string{"a.txt", "docs/b.txt"},
				},
				Names: map[string]string{"docs/b.txt": "sub/b.txt"},
			},
			setupAuth: testUser1Session,
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				setupServer1WithNode(t, serverRepo, nodeRepo, rbacRepo)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "filename contains path separators",
			wantCopyCalls:  new(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			nodeRepo := inmemory.NewNodeRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()
			fileService := tt.setupFileService()
			handler := NewHandler(serverRepo, nodeRepo, rbacService, fileService, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(serverRepo, nodeRepo, rbacRepo)
			}

			ctx := tt.setupAuth()

			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/file-manager/"+tt.serverID+"/paste", bytes.NewReader(body))
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"server": tt.serverID})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errorMsg, tt.wantError)
			}

			if tt.wantCopyCalls != nil {
				assert.Equal(t, *tt.wantCopyCalls, fileService.copyCalls, "unexpected Copy call count")
			}

			if tt.wantMoveCalls != nil {
				assert.Equal(t, *tt.wantMoveCalls, fileService.moveCalls, "unexpected Move call count")
			}

			if tt.validateResponse != nil {
				tt.validateResponse(t, w.Body.Bytes())
			}
		})
	}
}
