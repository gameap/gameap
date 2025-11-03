package updatefile

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
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

var testUser1 = domain.User{
	ID:    1,
	Login: "testuser",
	Email: "test@example.com",
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
	uploadStreamFunc func(
		ctx context.Context,
		node *domain.Node,
		filePath string,
		r io.Reader,
		size uint64,
		perms os.FileMode,
	) error
	getFileInfoFunc func(ctx context.Context, node *domain.Node, path string) (*daemon.FileDetails, error)
}

func (m *mockFileService) UploadStream(
	ctx context.Context,
	node *domain.Node,
	filePath string,
	r io.Reader,
	size uint64,
	perms os.FileMode,
) error {
	if m.uploadStreamFunc != nil {
		return m.uploadStreamFunc(ctx, node, filePath, r, size, perms)
	}

	return nil
}

func (m *mockFileService) GetFileInfo(ctx context.Context, node *domain.Node, path string) (*daemon.FileDetails, error) {
	if m.getFileInfoFunc != nil {
		return m.getFileInfoFunc(ctx, node, path)
	}

	return &daemon.FileDetails{
		Name:             "test.txt",
		Mime:             "text/plain; charset=utf-8",
		Size:             100,
		ModificationTime: 1761491739,
		Perm:             0o644,
		Type:             daemon.FileTypeFile,
	}, nil
}

func TestHandler_ServeHTTP(t *testing.T) {
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
			name:     "successful_file_update",
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
					uploadStreamFunc: func(
						_ context.Context,
						_ *domain.Node,
						filePath string,
						_ io.Reader,
						size uint64,
						perms os.FileMode,
					) error {
						assert.Equal(t, "/home/gameap/servers/test1/eula.txt", filePath)
						assert.Equal(t, uint64(18), size)
						assert.Equal(t, os.FileMode(0o644), perms)

						return nil
					},
					getFileInfoFunc: func(_ context.Context, _ *domain.Node, path string) (*daemon.FileDetails, error) {
						assert.Equal(t, "/home/gameap/servers/test1/eula.txt", path)

						return &daemon.FileDetails{
							Name:             "eula.txt",
							Mime:             "text/plain; charset=utf-8",
							Size:             19,
							ModificationTime: 1761491739,
							Perm:             0o644,
							Type:             daemon.FileTypeFile,
						}, nil
					},
				}
			},
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
				require.NoError(t, w.WriteField("path", ""))

				part, err := w.CreateFormFile("file", "eula.txt")
				require.NoError(t, err)
				_, err = part.Write([]byte("eula=true\ntest=yes"))
				require.NoError(t, err)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response updateFileResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Equal(t, "File updated!", response.Result.Message)
				assert.Equal(t, "eula.txt", response.File.Path)
				assert.Equal(t, uint64(19), response.File.Size)
				assert.Equal(t, "file", response.File.Type)
				assert.Equal(t, uint64(1761491739), response.File.Timestamp)
				assert.Equal(t, "public", response.File.Visibility)
				assert.Equal(t, "text/plain; charset=utf-8", response.File.Mimetype)
				assert.Equal(t, "eula.txt", response.File.Basename)
				assert.Equal(t, "", response.File.Dirname)
				assert.Equal(t, "txt", response.File.Extension)
				assert.Equal(t, "eula", response.File.Filename)
			},
		},
		{
			name:     "update_file_in_subdirectory",
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
					uploadStreamFunc: func(
						_ context.Context,
						_ *domain.Node,
						filePath string,
						_ io.Reader,
						_ uint64,
						_ os.FileMode,
					) error {
						assert.Equal(t, "/home/gameap/servers/test1/configs/server.cfg", filePath)

						return nil
					},
					getFileInfoFunc: func(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
						return &daemon.FileDetails{
							Name:             "server.cfg",
							Mime:             "text/plain",
							Size:             200,
							ModificationTime: 1761491739,
							Perm:             0o644,
							Type:             daemon.FileTypeFile,
						}, nil
					},
				}
			},
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
				require.NoError(t, w.WriteField("path", "configs"))

				part, err := w.CreateFormFile("file", "server.cfg")
				require.NoError(t, err)
				_, err = part.Write([]byte("hostname=Test Server"))
				require.NoError(t, err)
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response updateFileResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Equal(t, "configs/server.cfg", response.File.Path)
				assert.Equal(t, "configs", response.File.Dirname)
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
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "local"))
				require.NoError(t, w.WriteField("path", ""))

				part, err := w.CreateFormFile("file", "test.txt")
				require.NoError(t, err)
				_, err = part.Write([]byte("test"))
				require.NoError(t, err)
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "unsupported disk",
		},
		{
			name:     "no_file_uploaded",
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
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
				require.NoError(t, w.WriteField("path", ""))
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "no file uploaded",
		},
		{
			name:     "file_too_large",
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
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
				require.NoError(t, w.WriteField("path", ""))

				part, err := w.CreateFormFile("file", "large.bin")
				require.NoError(t, err)

				largeContent := make([]byte, maxUploadSize+1)
				_, err = part.Write(largeContent)
				require.NoError(t, err)
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "file exceeds maximum size",
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
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
				require.NoError(t, w.WriteField("path", "../../../etc"))

				part, err := w.CreateFormFile("file", "test.txt")
				require.NoError(t, err)
				_, err = part.Write([]byte("test"))
				require.NoError(t, err)
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "path contains invalid directory traversal",
		},
		{
			name:     "invalid_filename_with_directory_traversal",
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
					getFileInfoFunc: func(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
						return &daemon.FileDetails{
							Name:             "test.txt",
							Mime:             "text/plain",
							Size:             4,
							ModificationTime: 1761491739,
							Perm:             0o644,
							Type:             daemon.FileTypeFile,
						}, nil
					},
				}
			},
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
				require.NoError(t, w.WriteField("path", ""))

				part, err := w.CreateFormFile("file", "..%2Ftest.txt")
				require.NoError(t, err)
				_, err = part.Write([]byte("test"))
				require.NoError(t, err)
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "filename contains invalid directory traversal",
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
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
				require.NoError(t, w.WriteField("path", ""))

				part, err := w.CreateFormFile("file", "test.txt")
				require.NoError(t, err)
				_, err = part.Write([]byte("test"))
				require.NoError(t, err)
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "node not found",
		},
		{
			name:     "upload_stream_error",
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
					uploadStreamFunc: func(
						_ context.Context,
						_ *domain.Node,
						_ string,
						_ io.Reader,
						_ uint64,
						_ os.FileMode,
					) error {
						return errors.New("upload failed")
					},
				}
			},
			setupForm: func(w *multipart.Writer) {
				require.NoError(t, w.WriteField("disk", "server"))
				require.NoError(t, w.WriteField("path", ""))

				part, err := w.CreateFormFile("file", "test.txt")
				require.NoError(t, err)
				_, err = part.Write([]byte("test"))
				require.NoError(t, err)
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

			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			tt.setupForm(writer)
			require.NoError(t, writer.Close())

			req := httptest.NewRequest(http.MethodPost, "/api/file-manager/"+tt.serverID+"/update-file", body)
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

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid_relative_path",
			path:    "configs/server.cfg",
			wantErr: false,
		},
		{
			name:    "valid_single_directory",
			path:    "configs",
			wantErr: false,
		},
		{
			name:    "valid_root",
			path:    ".",
			wantErr: false,
		},
		{
			name:    "invalid_directory_traversal",
			path:    "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "invalid_path_with_double_dots",
			path:    "configs/../../etc",
			wantErr: true,
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

func TestValidateFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{
			name:     "valid_filename",
			filename: "test.txt",
			wantErr:  false,
		},
		{
			name:     "valid_filename_with_dots",
			filename: "server.properties.backup",
			wantErr:  false,
		},
		{
			name:     "empty_filename",
			filename: "",
			wantErr:  true,
		},
		{
			name:     "filename_with_directory_traversal",
			filename: "../test.txt",
			wantErr:  true,
		},
		{
			name:     "filename_with_forward_slash",
			filename: "dir/test.txt",
			wantErr:  true,
		},
		{
			name:     "filename_with_backslash",
			filename: "dir\\test.txt",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilename(tt.filename)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
