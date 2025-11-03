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
						assert.Equal(t, "/home/gameap/servers/test1/test.txt", filePath)
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
				callCount := 0
				expectedFiles := []struct {
					path string
					size uint64
				}{
					{path: "/home/gameap/servers/test1/file1.txt", size: 5},
					{path: "/home/gameap/servers/test1/file2.txt", size: 5},
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
						assert.Equal(t, "/home/gameap/servers/test1/configs/test.cfg", filePath)

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
					DSID:          999, // Non-existent node ID
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
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "node not found",
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
