package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"log/slog"

	"github.com/gameap/gameap/internal/api/filemanager/filemanagermime"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/daemon"
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

// permissiveTestMIME returns a Checker that accepts every default MIME plus
// archives and binary so existing upload tests (which use random/text
// bodies) keep passing while the new C-8 regression tests pin the
// rejection path explicitly. Production wiring uses the operator config.
func permissiveTestMIME() *filemanagermime.Checker {
	return filemanagermime.NewChecker(filemanagermime.Config{
		AllowArchives: true,
		AllowBinary:   true,
	})
}

// auditCapture is a concurrency-safe audit.Logger that records every event
// the handler emits (mirrors router_security_auditlog_test.go).
type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *auditCapture) snapshot() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]audit.Event(nil), a.events...)
}

func findEvent(events []audit.Event, t audit.EventType) (audit.Event, bool) {
	for _, e := range events {
		if e.Type == t {
			return e, true
		}
	}

	return audit.Event{}, false
}

func countEvents(events []audit.Event, t audit.EventType) int {
	n := 0
	for _, e := range events {
		if e.Type == t {
			n++
		}
	}

	return n
}

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
	uploadStreamFunc func(
		ctx context.Context,
		node *domain.Node,
		filePath string,
		r io.Reader,
		size uint64,
		perms os.FileMode,
	) error
}

func (m *mockFileService) UploadStream(
	ctx context.Context,
	node *domain.Node,
	filePath string,
	r io.Reader,
	size uint64,
	perms os.FileMode,
	_ daemon.OwnerOptions,
) error {
	if m.uploadStreamFunc != nil {
		return m.uploadStreamFunc(ctx, node, filePath, r, size, perms)
	}

	return nil
}

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		serverID         string
		setupAuth        func() context.Context
		setupRepo        func(*inmemory.ServerRepository, *inmemory.NodeRepository, *inmemory.RBACRepository)
		setupFileService func() *mockFileService
		setupForm        func(*multipart.Writer)
		expectedStatus   int
		wantError        string
		validateResponse func(*testing.T, []byte)
	}{
		{
			name:     "successful_single_file_upload",
			serverID: "1",
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
					uploadStreamFunc: func(
						_ context.Context,
						_ *domain.Node,
						filePath string,
						_ io.Reader,
						size uint64,
						perms os.FileMode,
					) error {
						assert.Equal(t, "/srv/gameap/servers/test1/test.txt", filePath)
						assert.Equal(t, uint64(12), size) // "test content" is 12 bytes
						assert.Equal(t, os.FileMode(0o644), perms)

						return nil
					},
				}
			},
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
				require.NoError(t, w.WriteField("path", ""))
				require.NoError(t, w.WriteField("overwrite", "0"))

				part, err := w.CreateFormFile("files[]", "test.txt")
				require.NoError(t, err)
				_, err = part.Write([]byte("test content"))
				require.NoError(t, err)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response uploadResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Equal(t, "All files uploaded!", response.Result.Message)
			},
		},
		{
			name:     "successful_multiple_files_upload",
			serverID: "1",
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
				expectedFiles := []struct {
					path string
					size uint64
				}{
					{path: "/srv/gameap/servers/test1/file1.txt", size: 5},
					{path: "/srv/gameap/servers/test1/file2.txt", size: 5},
				}

				return &mockFileService{
					uploadStreamFunc: func(
						_ context.Context,
						_ *domain.Node,
						filePath string,
						_ io.Reader,
						size uint64,
						_ os.FileMode,
					) error {
						require.Less(t, callCount, len(expectedFiles), "more calls than expected")
						assert.Equal(t, expectedFiles[callCount].path, filePath)
						assert.Equal(t, expectedFiles[callCount].size, size)
						callCount++

						return nil
					},
				}
			},
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
				require.NoError(t, w.WriteField("path", ""))
				require.NoError(t, w.WriteField("overwrite", "0"))

				part1, err := w.CreateFormFile("files[]", "file1.txt")
				require.NoError(t, err)
				_, err = part1.Write([]byte("file1"))
				require.NoError(t, err)

				part2, err := w.CreateFormFile("files[]", "file2.txt")
				require.NoError(t, err)
				_, err = part2.Write([]byte("file2"))
				require.NoError(t, err)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response uploadResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
			},
		},
		{
			name:     "upload_to_subdirectory",
			serverID: "1",
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
					uploadStreamFunc: func(
						_ context.Context,
						_ *domain.Node,
						filePath string,
						_ io.Reader,
						_ uint64,
						_ os.FileMode,
					) error {
						assert.Equal(t, "/srv/gameap/servers/test1/configs/test.cfg", filePath)

						return nil
					},
				}
			},
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
				require.NoError(t, w.WriteField("path", "configs"))
				require.NoError(t, w.WriteField("overwrite", "0"))

				part, err := w.CreateFormFile("files[]", "test.cfg")
				require.NoError(t, err)
				_, err = part.Write([]byte("config=value"))
				require.NoError(t, err)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response uploadResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
			},
		},
		{
			name:     "unsupported_disk",
			serverID: "1",
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
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "local"))
				require.NoError(t, w.WriteField("path", ""))
				require.NoError(t, w.WriteField("overwrite", "0"))

				part, err := w.CreateFormFile("files[]", "test.txt")
				require.NoError(t, err)
				_, err = part.Write([]byte("test"))
				require.NoError(t, err)
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "unsupported disk",
		},
		{
			name:     "no_files_uploaded",
			serverID: "1",
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
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
				require.NoError(t, w.WriteField("path", ""))
				require.NoError(t, w.WriteField("overwrite", "0"))
				// No files added
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "no files uploaded",
		},
		{
			name:     "user_not_authenticated",
			serverID: "1",
			//nolint:gocritic
			setupAuth: func() context.Context {
				return context.Background()
			},
			setupRepo: func(_ *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.RBACRepository) {
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:     "invalid_server_id",
			serverID: "invalid",
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
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
		},
		{
			name:     "server_not_found",
			serverID: "999",
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
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
		},
		{
			name:     "invalid_path_with_directory_traversal",
			serverID: "1",
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
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
				require.NoError(t, w.WriteField("path", "../../../etc"))
				require.NoError(t, w.WriteField("overwrite", "0"))

				part, err := w.CreateFormFile("files[]", "test.txt")
				require.NoError(t, err)
				_, err = part.Write([]byte("test"))
				require.NoError(t, err)
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "path contains invalid directory traversal",
		},
		{
			name:     "node_not_found",
			serverID: "1",
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
					DSID:          999, // Non-existent node ID
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
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "node not found",
		},
		{
			name:     "user_without_files_permission",
			serverID: "1",
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
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
				require.NoError(t, w.WriteField("path", ""))
				require.NoError(t, w.WriteField("overwrite", "0"))

				part, err := w.CreateFormFile("files[]", "test.txt")
				require.NoError(t, err)
				_, err = part.Write([]byte("test"))
				require.NoError(t, err)
			},
			expectedStatus: http.StatusForbidden,
			wantError:      "user does not have required permissions",
		},
		{
			name:     "admin_bypasses_files_permission",
			serverID: "1",
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
					uploadStreamFunc: func(
						_ context.Context,
						_ *domain.Node,
						_ string,
						_ io.Reader,
						_ uint64,
						_ os.FileMode,
					) error {
						return nil
					},
				}
			},
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
				require.NoError(t, w.WriteField("path", ""))
				require.NoError(t, w.WriteField("overwrite", "0"))

				part, err := w.CreateFormFile("files[]", "test.txt")
				require.NoError(t, err)
				_, err = part.Write([]byte("test"))
				require.NoError(t, err)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response uploadResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			serverRepo := inmemory.NewServerRepository()
			nodeRepo := inmemory.NewNodeRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()
			fileService := tt.setupFileService()
			handler := NewHandler(serverRepo, nodeRepo, rbacService, fileService, permissiveTestMIME(), responder, nil)

			if tt.setupRepo != nil {
				tt.setupRepo(serverRepo, nodeRepo, rbacRepo)
			}

			ctx := tt.setupAuth()

			// Build multipart form request
			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			tt.setupForm(writer)
			require.NoError(t, writer.Close())

			req := httptest.NewRequest(http.MethodPost, "/api/file-manager/"+tt.serverID+"/upload", body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
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

// ---------------------------------------------------------------------------
// Security audit-trail tests.
//
// OWASP API Security Top 10:2023:
//   - API8:2023 Security Misconfiguration — a file upload that succeeds
//     without being recorded is a detective-control gap (OWASP ASVS §7.2.1).
//     The event must be scoped to the exact server and attributed to the
//     acting principal.
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

func newUploadAuditServer() *domain.Server {
	now := time.Now()

	return &domain.Server{
		ID:        1,
		UID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		UUIDShort: "short1",
		Enabled:   true,
		Installed: 1,
		Name:      "Test Server 1",
		GameID:    "cs",
		DSID:      1,
		GameModID: 1,
		ServerIP:  "127.0.0.1",
		Dir:       "servers/test1",
		CreatedAt: &now,
		UpdatedAt: &now,
	}
}

func uploadAuditForm(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("disk", "server"))
	require.NoError(t, writer.WriteField("path", ""))
	require.NoError(t, writer.WriteField("overwrite", "0"))
	part, err := writer.CreateFormFile("files[]", "test.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte("test content"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	return body, writer.FormDataContentType()
}

// TestHandler_Audit_SuccessfulUploadIsRecorded covers OWASP API8:2023. A
// successful upload must emit exactly one file.upload event with outcome
// success, category file_op, the server as the scoped resource, and the
// acting user attributed as the actor.
func TestHandler_Audit_SuccessfulUploadIsRecorded(t *testing.T) {
	t.Parallel()

	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)

	require.NoError(t, serverRepo.Save(context.Background(), newUploadAuditServer()))
	serverRepo.AddUserServer(1, 1)
	allowUserFilesAbility(t, rbacRepo, 1, 1)
	node := testNode
	require.NoError(t, nodeRepo.Save(context.Background(), &node))

	recorder := &auditCapture{}
	handler := NewHandler(
		serverRepo, nodeRepo, rbacService, &mockFileService{}, permissiveTestMIME(), api.NewResponder(), recorder,
	)

	body, contentType := uploadAuditForm(t)
	req := httptest.NewRequest(http.MethodPost, "/api/file-manager/1/upload", body)
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(auth.ContextWithSession(context.Background(), &auth.Session{
		Login: "testuser",
		Email: "test@example.com",
		User:  &testUser1,
	}))
	req = mux.SetURLVars(req, map[string]string{"server": "1"})
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code, "upload must succeed; body=%s", w.Body.String())

	events := recorder.snapshot()
	require.Equal(t, 1, countEvents(events, audit.EventFileUpload),
		"exactly one file-upload event must be emitted per successful upload")

	ev, ok := findEvent(events, audit.EventFileUpload)
	require.True(t, ok, "a successful upload must leave a file.upload audit event")
	assert.Equal(t, audit.OutcomeSuccess, ev.Outcome, "a completed sensitive op records success")
	assert.Equal(t, audit.CategoryFileOp, ev.Category)
	assert.Equal(t, "server", ev.ResourceType, "a file op is scoped to its server")
	assert.Equal(t, "1", ev.ResourceID, "the targeted server id must be recorded")
	assert.Equal(t, "upload", ev.Action)
	assert.Equal(t, testUser1.ID, ev.ActorID, "the acting user must be attributed as the actor")
	assert.Equal(t, audit.AuthMethodSession, ev.AuthMethod)
}

// TestHandler_Audit_DeniedUploadDoesNotEmitFileUpload covers OWASP API8:2023.
// An upload refused by the per-server ability check must NOT emit a
// file.upload success event.
func TestHandler_Audit_DeniedUploadDoesNotEmitFileUpload(t *testing.T) {
	t.Parallel()

	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)

	require.NoError(t, serverRepo.Save(context.Background(), newUploadAuditServer()))
	// No files ability granted for this user/server.
	node := testNode
	require.NoError(t, nodeRepo.Save(context.Background(), &node))

	recorder := &auditCapture{}
	handler := NewHandler(
		serverRepo, nodeRepo, rbacService, &mockFileService{}, permissiveTestMIME(), api.NewResponder(), recorder,
	)

	body, contentType := uploadAuditForm(t)
	req := httptest.NewRequest(http.MethodPost, "/api/file-manager/1/upload", body)
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(auth.ContextWithSession(context.Background(), &auth.Session{
		Login: "testuser",
		Email: "test@example.com",
		User:  &testUser1,
	}))
	req = mux.SetURLVars(req, map[string]string{"server": "1"})
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.NotEqual(t, http.StatusOK, w.Code,
		"a user without the files ability must be denied; body=%s", w.Body.String())
	assert.Equal(t, 0, countEvents(recorder.snapshot(), audit.EventFileUpload),
		"a refused upload must not be recorded as a successful file.upload")
}

// uploadFormWithContent builds a multipart upload request whose single file
// part carries the given filename + body. Used by the C-8 MIME-validation
// regression tests below.
func uploadFormWithContent(t *testing.T, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("disk", "server"))
	require.NoError(t, writer.WriteField("path", ""))
	require.NoError(t, writer.WriteField("overwrite", "0"))
	part, err := writer.CreateFormFile("files[]", filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	return body, writer.FormDataContentType()
}

// runUploadWithMime drives a single upload through the handler with the
// supplied MIME checker and returns the recorder + audit capture.
func runUploadWithMime(t *testing.T, mimeChecker *filemanagermime.Checker, filename string, content []byte) (*httptest.ResponseRecorder, *auditCapture) {
	t.Helper()

	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)

	require.NoError(t, serverRepo.Save(context.Background(), newUploadAuditServer()))
	serverRepo.AddUserServer(1, 1)
	allowUserFilesAbility(t, rbacRepo, 1, 1)
	node := testNode
	require.NoError(t, nodeRepo.Save(context.Background(), &node))

	recorder := &auditCapture{}
	handler := NewHandler(
		serverRepo, nodeRepo, rbacService, &mockFileService{}, mimeChecker, api.NewResponder(), recorder,
	)

	body, contentType := uploadFormWithContent(t, filename, content)
	req := httptest.NewRequest(http.MethodPost, "/api/file-manager/1/upload", body)
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(auth.ContextWithSession(context.Background(), &auth.Session{
		Login: "testuser", User: &testUser1,
	}))
	req = mux.SetURLVars(req, map[string]string{"server": "1"})

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	return w, recorder
}

// TestHandler_C8_HTMLAsPNG_RejectedAndAudited — OWASP API6:2023 — ASVS_L2
// §C-8. An HTML payload renamed to "logo.png" must be rejected at upload
// time on the MIME sniff, even though the filename allow-list accepts it.
// The audit record captures the detected MIME so an operator can spot the
// attempted bypass.
func TestHandler_C8_HTMLAsPNG_RejectedAndAudited(t *testing.T) {
	t.Parallel()

	mimeChecker := filemanagermime.NewChecker(filemanagermime.Config{})

	htmlBody := []byte(`<!DOCTYPE html><html><body><script>alert("xss")</script></body></html>`)
	w, recorder := runUploadWithMime(t, mimeChecker, "logo.png", htmlBody)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code,
		"HTML masquerading as PNG must be rejected; body=%s", w.Body.String())

	events := recorder.snapshot()
	require.Len(t, events, 1, "exactly one audit event must be recorded for the rejected upload")

	got := events[0]
	assert.Equal(t, audit.EventFileUpload, got.Type)

	// The detected MIME must be carried in the event metadata so an
	// operator can see what was rejected.
	require.NotEmpty(t, got.Extra)
	detected := lookupExtraString(got.Extra, "detected_mime")
	assert.Contains(t, detected, "text/html",
		"detected MIME for HTML-as-PNG must surface text/html")

	reason := lookupExtraString(got.Extra, "reason")
	assert.Equal(t, "mime_not_allowed", reason,
		"audit event must carry the stable rejection reason")
}

// lookupExtraString fetches a string-valued attribute from an event's Extra
// slice. Returns "" when the key is absent or non-string so test failure
// messages are clear.
func lookupExtraString(extra []slog.Attr, key string) string {
	for _, a := range extra {
		if a.Key == key {
			return a.Value.String()
		}
	}

	return ""
}

// TestHandler_C8_ValidPNG_Accepted — OWASP API6:2023 — a real PNG must
// pass the MIME gate so the regression does not block legitimate uploads.
func TestHandler_C8_ValidPNG_Accepted(t *testing.T) {
	t.Parallel()

	mimeChecker := filemanagermime.NewChecker(filemanagermime.Config{})

	// Minimal PNG magic header (8 bytes) — enough for DetectContentType to
	// classify the payload as image/png.
	pngHeader := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x00}

	w, _ := runUploadWithMime(t, mimeChecker, "real.png", pngHeader)

	assert.Equal(t, http.StatusOK, w.Code,
		"a real PNG must pass the MIME gate; body=%s", w.Body.String())
}

// TestHandler_C8_PlainTextConfig_Accepted — OWASP API6:2023 — the defaults
// must let game-config files through. DetectContentType returns text/plain
// for an ASCII config, which is on the default allowlist.
func TestHandler_C8_PlainTextConfig_Accepted(t *testing.T) {
	t.Parallel()

	mimeChecker := filemanagermime.NewChecker(filemanagermime.Config{})

	cfg := []byte("server_name=test\nmaxplayers=16\nrcon_password=...\n")

	w, _ := runUploadWithMime(t, mimeChecker, "server.cfg", cfg)

	assert.Equal(t, http.StatusOK, w.Code,
		"plain-text config must pass the MIME gate; body=%s", w.Body.String())
}

// TestHandler_C8_ZIP_RejectedByDefault — OWASP API6:2023 — an archive
// must be refused under the default (deny-by-default) configuration.
// Operators that legitimately need archives flip AllowArchives.
func TestHandler_C8_ZIP_RejectedByDefault(t *testing.T) {
	t.Parallel()

	mimeChecker := filemanagermime.NewChecker(filemanagermime.Config{})

	// PK\x03\x04 — ZIP signature.
	zipHeader := []byte{0x50, 0x4b, 0x03, 0x04, 0x14, 0x00, 0x00, 0x00, 0x08, 0x00}

	w, _ := runUploadWithMime(t, mimeChecker, "pack.zip", zipHeader)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code,
		"ZIP uploads must be refused by default; body=%s", w.Body.String())
}

// TestHandler_C8_ZIP_AcceptedWhenArchivesAllowed — OWASP API6:2023 — the
// operator override flips the same upload from 415 → 200, proving the
// allow-list is config-driven rather than hard-coded.
func TestHandler_C8_ZIP_AcceptedWhenArchivesAllowed(t *testing.T) {
	t.Parallel()

	mimeChecker := filemanagermime.NewChecker(filemanagermime.Config{AllowArchives: true})

	zipHeader := []byte{0x50, 0x4b, 0x03, 0x04, 0x14, 0x00, 0x00, 0x00, 0x08, 0x00}

	w, _ := runUploadWithMime(t, mimeChecker, "pack.zip", zipHeader)

	assert.Equal(t, http.StatusOK, w.Code,
		"ZIP uploads must succeed when AllowArchives=true; body=%s", w.Body.String())
}
