// OWASP API Top 10:2023 — API1:2023 Broken Object Level Authorization.
// The rename oldName/newName fields are attacker-controlled; without strict
// filename validation a caller could traverse out of the server data
// directory (path/filename traversal) and move objects belonging to other
// tenants. These tests assert both name fields are validated with
// filemanagerpath.ValidateFilename (rejects "", "..", path separators).
package rename

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
	"github.com/pkg/errors"
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
	moveFunc func(ctx context.Context, node *domain.Node, source, destination string) error
}

func (m *mockFileService) Move(
	ctx context.Context,
	node *domain.Node,
	source,
	destination string,
) error {
	if m.moveFunc != nil {
		return m.moveFunc(ctx, node, source, destination)
	}

	return nil
}

// TestHandler_ServeHTTP — OWASP API Top 10:2023 API1:2023 Broken Object Level
// Authorization. The oldName/newName cases (empty/"..", separators) prove the
// handler rejects names that could escape the per-server directory; a name
// that previously passed because it contained a "/" subpath is now rejected.
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
		validateResponse func(*testing.T, []byte)
	}{
		{
			name:     "successful_file_rename",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "test.txt",
				NewName: "renamed.txt",
				Type:    "file",
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
					moveFunc: func(_ context.Context, _ *domain.Node, source, destination string) error {
						assert.Equal(t, "/srv/gameap/servers/test1/test.txt", source)
						assert.Equal(t, "/srv/gameap/servers/test1/renamed.txt", destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response renameResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Equal(t, "Renamed!", response.Result.Message)
			},
		},
		{
			name:     "successful_directory_rename",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "logs",
				NewName: "old-logs",
				Type:    "dir",
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
					moveFunc: func(_ context.Context, _ *domain.Node, source, destination string) error {
						assert.Equal(t, "/srv/gameap/servers/test1/logs", source)
						assert.Equal(t, "/srv/gameap/servers/test1/old-logs", destination)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response renameResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Equal(t, "Renamed!", response.Result.Message)
			},
		},
		{
			// Renaming a file inside a subdirectory is no longer accepted:
			// oldName/newName must be plain file names so an attacker cannot
			// supply a "/"-bearing path that escapes the server directory.
			name:     "subdirectory_in_name_rejected",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "some-dir/ca.crt",
				NewName: "some-dir/ca2.crt",
				Type:    "file",
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
			wantError:      "filename contains path separators",
		},
		{
			name:     "unsupported_disk",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "local",
				OldName: "test.txt",
				NewName: "renamed.txt",
				Type:    "file",
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
			name:     "missing_oldName",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "",
				NewName: "renamed.txt",
				Type:    "file",
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
			wantError:      "oldName is required",
		},
		{
			name:     "missing_newName",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "test.txt",
				NewName: "",
				Type:    "file",
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
			wantError:      "newName is required",
		},
		{
			name:     "invalid_type",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "test.txt",
				NewName: "renamed.txt",
				Type:    "invalid",
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
			wantError:      "invalid type",
		},
		{
			name:     "user_not_authenticated",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "test.txt",
				NewName: "renamed.txt",
				Type:    "file",
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
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "test.txt",
				NewName: "renamed.txt",
				Type:    "file",
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
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "test.txt",
				NewName: "renamed.txt",
				Type:    "file",
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
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "test.txt",
				NewName: "renamed.txt",
				Type:    "file",
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
			name:     "admin_can_rename_on_any_server",
			serverID: "2",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "test.txt",
				NewName: "renamed.txt",
				Type:    "file",
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
					moveFunc: func(_ context.Context, _ *domain.Node, _ string, _ string) error {
						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response renameResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
			},
		},
		{
			name:     "invalid_path_with_directory_traversal_in_oldName",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "../../../etc/passwd",
				NewName: "renamed.txt",
				Type:    "file",
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
			wantError:      "filename contains invalid directory traversal",
		},
		{
			name:     "invalid_path_with_directory_traversal_in_newName",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "test.txt",
				NewName: "../../../etc/passwd",
				Type:    "file",
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
			wantError:      "filename contains invalid directory traversal",
		},
		{
			name:     "newName_with_path_separator_rejected",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "valid.txt",
				NewName: "a/b",
				Type:    "file",
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
			wantError:      "filename contains path separators",
		},
		{
			name:     "newName_equal_to_double_dot_rejected",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "valid.txt",
				NewName: "..",
				Type:    "file",
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
			wantError:      "filename contains invalid directory traversal",
		},
		{
			name:     "oldName_with_backslash_rejected",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "a\\b",
				NewName: "renamed.txt",
				Type:    "file",
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
			wantError:      "filename contains path separators",
		},
		{
			// Empty oldName/newName is rejected by validateRequest before the
			// filename check, but the rejection (a name field cannot be empty)
			// is still part of the name-validation contract under test.
			name:     "empty_newName_rejected",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "valid.txt",
				NewName: "",
				Type:    "file",
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
			wantError:      "newName is required",
		},
		{
			name:     "node_not_found",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "test.txt",
				NewName: "renamed.txt",
				Type:    "file",
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
			name:     "daemon_file_service_error",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "test.txt",
				NewName: "renamed.txt",
				Type:    "file",
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
					moveFunc: func(_ context.Context, _ *domain.Node, _, _ string) error {
						return errors.New("daemon connection error")
					},
				}
			},
			expectedStatus: http.StatusInternalServerError,
			wantError:      "Internal Server Error",
		},
		{
			name:     "user_without_files_permission",
			serverID: "1",
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "test.txt",
				NewName: "renamed.txt",
				Type:    "file",
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
			requestBody: renameRequest{
				Disk:    "server",
				OldName: "test.txt",
				NewName: "renamed.txt",
				Type:    "file",
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
					moveFunc: func(_ context.Context, _ *domain.Node, _ string, _ string) error {
						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response renameResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
			},
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

			req := httptest.NewRequest(http.MethodPost, "/api/file-manager/"+tt.serverID+"/rename", bytes.NewReader(body))
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

			if tt.validateResponse != nil {
				tt.validateResponse(t, w.Body.Bytes())
			}
		})
	}
}
