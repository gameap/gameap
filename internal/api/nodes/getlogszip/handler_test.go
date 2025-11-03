package getlogszip

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testUser1 = domain.User{
	ID:    1,
	Login: "admin",
	Email: "admin@example.com",
}

type mockFileService struct {
	readDirFunc        func(ctx context.Context, node *domain.Node, directory string) ([]*daemon.FileInfo, error)
	downloadStreamFunc func(ctx context.Context, node *domain.Node, filePath string) (io.ReadCloser, error)
}

func (m *mockFileService) ReadDir(
	ctx context.Context,
	node *domain.Node,
	directory string,
) ([]*daemon.FileInfo, error) {
	if m.readDirFunc != nil {
		return m.readDirFunc(ctx, node, directory)
	}

	return nil, nil
}

func (m *mockFileService) DownloadStream(
	ctx context.Context,
	node *domain.Node,
	filePath string,
) (io.ReadCloser, error) {
	if m.downloadStreamFunc != nil {
		return m.downloadStreamFunc(ctx, node, filePath)
	}

	return io.NopCloser(bytes.NewReader([]byte{})), nil
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		nodeID         string
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.NodeRepository)
		setupMock      func() *mockFileService
		expectedStatus int
		wantError      string
		expectZip      bool
	}{
		{
			name:   "successful logs download",
			nodeID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(nodesRepo *inmemory.NodeRepository) {
				now := time.Now()

				node := &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "test-node",
					OS:                  "linux",
					Location:            "Montenegro",
					IPs:                 []string{"172.18.0.5"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "172.18.0.5",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				require.NoError(t, nodesRepo.Save(context.Background(), node))
			},
			setupMock: func() *mockFileService {
				return &mockFileService{
					readDirFunc: func(_ context.Context, _ *domain.Node, _ string) ([]*daemon.FileInfo, error) {
						return []*daemon.FileInfo{
							{
								Name:         "daemon.log",
								Size:         1024,
								TimeModified: uint64(time.Now().Unix()),
								Type:         daemon.FileTypeFile,
								Perm:         0644,
							},
							{
								Name:         "error.log",
								Size:         512,
								TimeModified: uint64(time.Now().Unix()),
								Type:         daemon.FileTypeFile,
								Perm:         0644,
							},
						}, nil
					},
					downloadStreamFunc: func(_ context.Context, _ *domain.Node, filePath string) (io.ReadCloser, error) {
						content := "log content for " + filePath

						return io.NopCloser(bytes.NewReader([]byte(content))), nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			expectZip:      true,
		},
		{
			name:   "windows node logs download",
			nodeID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(nodesRepo *inmemory.NodeRepository) {
				now := time.Now()

				node := &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "windows-node",
					OS:                  "windows",
					Location:            "US",
					IPs:                 []string{"192.168.1.1"},
					WorkPath:            "C:/gameap",
					GdaemonHost:         "192.168.1.1",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				require.NoError(t, nodesRepo.Save(context.Background(), node))
			},
			setupMock: func() *mockFileService {
				return &mockFileService{
					readDirFunc: func(_ context.Context, _ *domain.Node, directory string) ([]*daemon.FileInfo, error) {
						assert.Equal(t, "C:/gameap/daemon/logs", directory)

						return []*daemon.FileInfo{
							{
								Name:         "daemon.log",
								Size:         1024,
								TimeModified: uint64(time.Now().Unix()),
								Type:         daemon.FileTypeFile,
								Perm:         0644,
							},
						}, nil
					},
					downloadStreamFunc: func(_ context.Context, _ *domain.Node, _ string) (io.ReadCloser, error) {
						return io.NopCloser(bytes.NewReader([]byte("windows log content"))), nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			expectZip:      true,
		},
		{
			name:   "node not found",
			nodeID: "999",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(_ *inmemory.NodeRepository) {},
			setupMock: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "node not found",
			expectZip:      false,
		},
		{
			name:      "user not authenticated",
			nodeID:    "1",
			setupRepo: func(_ *inmemory.NodeRepository) {},
			setupMock: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectZip:      false,
		},
		{
			name:   "invalid node id",
			nodeID: "invalid",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(_ *inmemory.NodeRepository) {},
			setupMock: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid node id",
			expectZip:      false,
		},
		{
			name:   "empty log directory",
			nodeID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(nodesRepo *inmemory.NodeRepository) {
				now := time.Now()

				node := &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "test-node",
					OS:                  "linux",
					Location:            "Montenegro",
					IPs:                 []string{"172.18.0.5"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "172.18.0.5",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				require.NoError(t, nodesRepo.Save(context.Background(), node))
			},
			setupMock: func() *mockFileService {
				return &mockFileService{
					readDirFunc: func(_ context.Context, _ *domain.Node, _ string) ([]*daemon.FileInfo, error) {
						return []*daemon.FileInfo{}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			expectZip:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodesRepo := inmemory.NewNodeRepository()
			responder := api.NewResponder()
			mockFS := tt.setupMock()
			handler := NewHandler(nodesRepo, mockFS, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(nodesRepo)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodGet, "/api/dedicated_servers/"+tt.nodeID+"/logs.zip", nil)
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"id": tt.nodeID})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.wantError != "" {
				assert.Contains(t, w.Body.String(), tt.wantError)
			}

			if tt.expectZip {
				assert.Equal(t, "application/zip", w.Header().Get("Content-Type"))
				assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
				assert.Contains(t, w.Header().Get("Content-Disposition"), "logs.zip")

				zipReader, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
				require.NoError(t, err)

				switch tt.name {
				case "empty log directory":
					assert.Len(t, zipReader.File, 0)
				case "successful logs download":
					require.Len(t, zipReader.File, 2)

					var foundDaemonLog, foundErrorLog bool
					for _, file := range zipReader.File {
						if file.Name == "daemon_logs/daemon.log" {
							foundDaemonLog = true
						}
						if file.Name == "daemon_logs/error.log" {
							foundErrorLog = true
						}
					}

					assert.True(t, foundDaemonLog, "daemon.log should be in the zip")
					assert.True(t, foundErrorLog, "error.log should be in the zip")
				}
			}
		})
	}
}

func TestHandler_NewHandler(t *testing.T) {
	nodesRepo := inmemory.NewNodeRepository()
	mockFS := &mockFileService{}
	responder := api.NewResponder()

	handler := NewHandler(nodesRepo, mockFS, responder)

	require.NotNil(t, handler)
	assert.Equal(t, nodesRepo, handler.nodesRepo)
	assert.Equal(t, mockFS, handler.daemonFiles)
	assert.Equal(t, responder, handler.responder)
}

func TestHandler_ZipContent(t *testing.T) {
	nodesRepo := inmemory.NewNodeRepository()
	responder := api.NewResponder()

	now := time.Now()
	node := &domain.Node{
		ID:                  1,
		Enabled:             true,
		Name:                "test-node",
		OS:                  "linux",
		Location:            "Montenegro",
		IPs:                 []string{"172.18.0.5"},
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "172.18.0.5",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "test-key",
		GdaemonServerCert:   "certs/root.crt",
		ClientCertificateID: 1,
		PreferInstallMethod: "auto",
		CreatedAt:           &now,
		UpdatedAt:           &now,
	}

	require.NoError(t, nodesRepo.Save(context.Background(), node))

	mockFS := &mockFileService{
		readDirFunc: func(_ context.Context, _ *domain.Node, _ string) ([]*daemon.FileInfo, error) {
			return []*daemon.FileInfo{
				{
					Name:         "test.log",
					Size:         1024,
					TimeModified: uint64(time.Now().Unix()),
					Type:         daemon.FileTypeFile,
					Perm:         0644,
				},
			}, nil
		},
		downloadStreamFunc: func(_ context.Context, _ *domain.Node, filePath string) (io.ReadCloser, error) {
			testContent := "This is test log content from " + filePath

			return io.NopCloser(bytes.NewReader([]byte(testContent))), nil
		},
	}

	handler := NewHandler(nodesRepo, mockFS, responder)

	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/dedicated_servers/1/logs.zip", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	zipReader, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	require.NoError(t, err)
	require.Len(t, zipReader.File, 1)

	file := zipReader.File[0]
	assert.Equal(t, "daemon_logs/test.log", file.Name)

	rc, err := file.Open()
	require.NoError(t, err)
	defer rc.Close()

	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Contains(t, string(content), "This is test log content")
}

func TestHandler_SkipDirectories(t *testing.T) {
	nodesRepo := inmemory.NewNodeRepository()
	responder := api.NewResponder()

	now := time.Now()
	node := &domain.Node{
		ID:                  1,
		Enabled:             true,
		Name:                "test-node",
		OS:                  "linux",
		Location:            "Montenegro",
		IPs:                 []string{"172.18.0.5"},
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "172.18.0.5",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "test-key",
		GdaemonServerCert:   "certs/root.crt",
		ClientCertificateID: 1,
		PreferInstallMethod: "auto",
		CreatedAt:           &now,
		UpdatedAt:           &now,
	}

	require.NoError(t, nodesRepo.Save(context.Background(), node))

	mockFS := &mockFileService{
		readDirFunc: func(_ context.Context, _ *domain.Node, _ string) ([]*daemon.FileInfo, error) {
			return []*daemon.FileInfo{
				{
					Name:         "archive",
					Size:         0,
					TimeModified: uint64(time.Now().Unix()),
					Type:         daemon.FileTypeDir,
					Perm:         0755,
				},
				{
					Name:         "test.log",
					Size:         1024,
					TimeModified: uint64(time.Now().Unix()),
					Type:         daemon.FileTypeFile,
					Perm:         0644,
				},
			}, nil
		},
		downloadStreamFunc: func(_ context.Context, _ *domain.Node, _ string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("file content"))), nil
		},
	}

	handler := NewHandler(nodesRepo, mockFS, responder)

	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/dedicated_servers/1/logs.zip", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	zipReader, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	require.NoError(t, err)

	require.Len(t, zipReader.File, 1)
	assert.Equal(t, "daemon_logs/test.log", zipReader.File[0].Name)
}
