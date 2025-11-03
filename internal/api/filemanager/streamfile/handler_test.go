package streamfile

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fakeVideoContent = "fake video content"

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
	WorkPath:            "/var/gameap",
	GdaemonServerCert:   "test-cert",
	ClientCertificateID: 1,
}

type mockFileService struct {
	downloadStreamFunc func(ctx context.Context, node *domain.Node, filePath string) (io.ReadCloser, error)
	getFileInfoFunc    func(ctx context.Context, node *domain.Node, path string) (*daemon.FileDetails, error)
}

func (m *mockFileService) DownloadStream(
	ctx context.Context,
	node *domain.Node,
	filePath string,
) (io.ReadCloser, error) {
	if m.downloadStreamFunc != nil {
		return m.downloadStreamFunc(ctx, node, filePath)
	}

	return nil, errors.New("not implemented")
}

func (m *mockFileService) GetFileInfo(
	ctx context.Context,
	node *domain.Node,
	path string,
) (*daemon.FileDetails, error) {
	if m.getFileInfoFunc != nil {
		return m.getFileInfoFunc(ctx, node, path)
	}

	return nil, errors.New("not implemented")
}

type mockReadCloser struct {
	io.Reader

	closeFunc func() error
}

func (m *mockReadCloser) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}

	return nil
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name             string
		serverID         string
		disk             string
		path             string
		setupAuth        func() context.Context
		setupRepo        func(*inmemory.ServerRepository, *inmemory.NodeRepository, *inmemory.RBACRepository)
		setupFileService func() *mockFileService
		expectedStatus   int
		wantError        string
		validateResponse func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:     "successful_video_stream",
			serverID: "1",
			disk:     "server",
			path:     "videos/demo.mp4",
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
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
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
					Dir:           "/home/gameap/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					getFileInfoFunc: func(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
						return &daemon.FileDetails{
							Name:             "demo.mp4",
							Mime:             "video/mp4",
							Size:             1024000,
							ModificationTime: 1609459200,
							Type:             daemon.FileTypeFile,
						}, nil
					},
					downloadStreamFunc: func(_ context.Context, _ *domain.Node, _ string) (io.ReadCloser, error) {
						return &mockReadCloser{Reader: strings.NewReader(fakeVideoContent)}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()

				assert.Equal(t, "video/mp4", w.Header().Get("Content-Type"))
				assert.Equal(t, "inline; filename=\"demo.mp4\"", w.Header().Get("Content-Disposition"))
				assert.Equal(t, "bytes", w.Header().Get("Accept-Ranges"))
				assert.Equal(t, "1024000", w.Header().Get("Content-Length"))
			},
		},
		{
			name:     "successful_audio_stream",
			serverID: "1",
			disk:     "server",
			path:     "sounds/music.mp3",
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
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
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
					Dir:           "/home/gameap/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					getFileInfoFunc: func(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
						return &daemon.FileDetails{
							Name:             "music.mp3",
							Mime:             "audio/mpeg",
							Size:             512000,
							ModificationTime: 1609459200,
							Type:             daemon.FileTypeFile,
						}, nil
					},
					downloadStreamFunc: func(_ context.Context, _ *domain.Node, _ string) (io.ReadCloser, error) {
						content := "fake audio content"

						return &mockReadCloser{Reader: strings.NewReader(content)}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()

				assert.Equal(t, "audio/mpeg", w.Header().Get("Content-Type"))
				assert.Equal(t, "inline; filename=\"music.mp3\"", w.Header().Get("Content-Disposition"))
				assert.Equal(t, "bytes", w.Header().Get("Accept-Ranges"))
				assert.Equal(t, "512000", w.Header().Get("Content-Length"))
			},
		},
		{
			name:     "non_ascii_filename_converted",
			serverID: "1",
			disk:     "server",
			path:     "vidéo.mp4",
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
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
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
					Dir:           "/home/gameap/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					getFileInfoFunc: func(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
						return &daemon.FileDetails{
							Name:             "vidéo.mp4",
							Mime:             "video/mp4",
							Size:             2048000,
							ModificationTime: 1609459200,
							Type:             daemon.FileTypeFile,
						}, nil
					},
					downloadStreamFunc: func(_ context.Context, _ *domain.Node, _ string) (io.ReadCloser, error) {
						return &mockReadCloser{Reader: strings.NewReader(fakeVideoContent)}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()

				assert.Equal(t, "video/mp4", w.Header().Get("Content-Type"))
				assert.Equal(t, "inline; filename=\"video.mp4\"", w.Header().Get("Content-Disposition"))
				assert.Equal(t, "bytes", w.Header().Get("Accept-Ranges"))
			},
		},
		{
			name:     "fallback_to_extension_mime_type",
			serverID: "1",
			disk:     "server",
			path:     "video.avi",
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
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
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
					Dir:           "/home/gameap/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					getFileInfoFunc: func(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
						return &daemon.FileDetails{
							Name:             "video.avi",
							Mime:             "",
							Size:             1536000,
							ModificationTime: 1609459200,
							Type:             daemon.FileTypeFile,
						}, nil
					},
					downloadStreamFunc: func(_ context.Context, _ *domain.Node, _ string) (io.ReadCloser, error) {
						return &mockReadCloser{Reader: strings.NewReader(fakeVideoContent)}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()

				contentType := w.Header().Get("Content-Type")
				assert.Contains(t, []string{"video/x-msvideo", "video/vnd.avi"}, contentType)
				assert.Equal(t, "bytes", w.Header().Get("Accept-Ranges"))
			},
		},
		{
			name:     "disk_parameter_required",
			serverID: "1",
			disk:     "",
			path:     "video.mp4",
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
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
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
					Dir:           "/home/gameap/servers/test1",
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
			expectedStatus: http.StatusBadRequest,
			wantError:      "disk parameter is required",
		},
		{
			name:     "path_parameter_required",
			serverID: "1",
			disk:     "server",
			path:     "",
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
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
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
					Dir:           "/home/gameap/servers/test1",
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
			expectedStatus: http.StatusBadRequest,
			wantError:      "path parameter is required",
		},
		{
			name:     "unsupported_disk",
			serverID: "1",
			disk:     "local",
			path:     "video.mp4",
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
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
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
					Dir:           "/home/gameap/servers/test1",
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
			expectedStatus: http.StatusBadRequest,
			wantError:      "unsupported disk",
		},
		{
			name:      "user_not_authenticated",
			serverID:  "1",
			disk:      "server",
			path:      "video.mp4",
			setupAuth: context.Background,
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
			disk:     "server",
			path:     "video.mp4",
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
			disk:     "server",
			path:     "video.mp4",
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
			name:     "invalid_path_with_directory_traversal",
			serverID: "1",
			disk:     "server",
			path:     "../../../etc/passwd",
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
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
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
					Dir:           "/home/gameap/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

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
			disk:     "server",
			path:     "video.mp4",
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
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
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
					Dir:           "/home/gameap/servers/test1",
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
			expectedStatus: http.StatusNotFound,
			wantError:      "node not found",
		},
		{
			name:     "admin_can_access_any_server",
			serverID: "2",
			disk:     "server",
			path:     "video.mp4",
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
					UUID:          uuid.MustParse("22222222-2222-2222-2222-222222222222"),
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
					Dir:           "/home/gameap/servers/test2",
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
					getFileInfoFunc: func(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
						return &daemon.FileDetails{
							Name:             "video.mp4",
							Mime:             "video/mp4",
							Size:             2048000,
							ModificationTime: 1609459200,
							Type:             daemon.FileTypeFile,
						}, nil
					},
					downloadStreamFunc: func(_ context.Context, _ *domain.Node, _ string) (io.ReadCloser, error) {
						return &mockReadCloser{Reader: strings.NewReader(fakeVideoContent)}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()

				assert.Equal(t, "video/mp4", w.Header().Get("Content-Type"))
				assert.Equal(t, "bytes", w.Header().Get("Accept-Ranges"))
			},
		},
		{
			name:     "get_file_info_fails",
			serverID: "1",
			disk:     "server",
			path:     "nonexistent.mp4",
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
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
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
					Dir:           "/home/gameap/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					getFileInfoFunc: func(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
						return nil, errors.New("file not found on daemon")
					},
				}
			},
			expectedStatus: http.StatusInternalServerError,
			wantError:      "Internal Server Error",
		},
		{
			name:     "daemon_download_fails",
			serverID: "1",
			disk:     "server",
			path:     "nonexistent.mp4",
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
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
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
					Dir:           "/home/gameap/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					getFileInfoFunc: func(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
						return &daemon.FileDetails{
							Name:             "nonexistent.mp4",
							Mime:             "video/mp4",
							Size:             0,
							ModificationTime: 1609459200,
							Type:             daemon.FileTypeFile,
						}, nil
					},
					downloadStreamFunc: func(_ context.Context, _ *domain.Node, _ string) (io.ReadCloser, error) {
						return nil, errors.New("file not found on daemon")
					},
				}
			},
			expectedStatus: http.StatusInternalServerError,
			wantError:      "Internal Server Error",
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

			baseURL := "/api/file-manager/" + tt.serverID + "/stream-file"
			query := url.Values{}
			if tt.disk != "" {
				query.Add("disk", tt.disk)
			}
			if tt.path != "" {
				query.Add("path", tt.path)
			}
			fullURL := baseURL
			if len(query) > 0 {
				fullURL += "?" + query.Encode()
			}

			req := httptest.NewRequest(http.MethodGet, fullURL, nil)
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"server": tt.serverID})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.wantError != "" {
				body := w.Body.String()
				assert.Contains(t, body, tt.wantError)
			}

			if tt.validateResponse != nil {
				tt.validateResponse(t, w)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid_relative_path",
			path:    "videos/demo.mp4",
			wantErr: false,
		},
		{
			name:    "valid_single_file",
			path:    "video.mp4",
			wantErr: false,
		},
		{
			name:    "invalid_directory_traversal_with_dots",
			path:    "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "invalid_path_with_double_dots",
			path:    "videos/../../etc",
			wantErr: true,
		},
		{
			name:    "invalid_just_double_dots",
			path:    "..",
			wantErr: true,
		},
		{
			name:    "invalid_double_dots_at_start",
			path:    "../videos",
			wantErr: true,
		},
		{
			name:    "valid_path_with_dots_in_filename",
			path:    "config/video.1.0.mp4",
			wantErr: false,
		},
		{
			name:    "valid_nested_path",
			path:    "media/videos/2024/demo.mp4",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConvertToASCII(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{
			name:     "already_ascii",
			filename: "video.mp4",
			want:     "video.mp4",
		},
		{
			name:     "french_accents",
			filename: "vidéo.mp4",
			want:     "video.mp4",
		},
		{
			name:     "spanish_characters",
			filename: "película.mp4",
			want:     "pelicula.mp4",
		},
		{
			name:     "german_umlauts",
			filename: "über.mp4",
			want:     "uber.mp4",
		},
		{
			name:     "mixed_characters",
			filename: "café-résumé.mp4",
			want:     "cafe-resume.mp4",
		},
		{
			name:     "cyrillic_to_underscore",
			filename: "видео.mp4",
			want:     "_____.mp4",
		},
		{
			name:     "chinese_to_underscore",
			filename: "视频.mp4",
			want:     "__.mp4",
		},
		{
			name:     "uppercase_accents",
			filename: "CAFÉ.MP4",
			want:     "CAFE.MP4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertToASCII(tt.filename)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetContentTypeFromExtension(t *testing.T) {
	tests := []struct {
		name             string
		filename         string
		wantContent      string
		acceptedContents []string
	}{
		{
			name:        "mp4_video",
			filename:    "video.mp4",
			wantContent: "video/mp4",
		},
		{
			name:        "mp3_audio",
			filename:    "music.mp3",
			wantContent: "audio/mpeg",
		},
		{
			name:             "avi_video",
			filename:         "video.avi",
			acceptedContents: []string{"video/x-msvideo", "video/vnd.avi"},
		},
		{
			name:        "mkv_video",
			filename:    "video.mkv",
			wantContent: "video/x-matroska",
		},
		{
			name:        "webm_video",
			filename:    "video.webm",
			wantContent: "video/webm",
		},
		{
			name:        "ogg_audio",
			filename:    "audio.ogg",
			wantContent: "audio/ogg",
		},
		{
			name:             "unknown_extension",
			filename:         "file.xyz",
			acceptedContents: []string{"chemical/x-xyz", "application/octet-stream"},
		},
		{
			name:        "no_extension",
			filename:    "video",
			wantContent: "application/octet-stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentType := getContentTypeFromExtension(tt.filename)
			if len(tt.acceptedContents) > 0 {
				assert.Contains(t, tt.acceptedContents, contentType)
			} else {
				assert.Equal(t, tt.wantContent, contentType)
			}
		})
	}
}
