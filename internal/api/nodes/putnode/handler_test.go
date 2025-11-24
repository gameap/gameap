package putnode

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validCertPEM = `-----BEGIN CERTIFICATE-----
MIIDBDCCAewCEGZ4yqqHhhnItdDl32wOqxUwDQYJKoZIhvcNAQELBQAwMjELMAkG
A1UEBhMCUlUxDzANBgNVBAoMBkdhbWVBUDESMBAGA1UEAwwJR2FtZUFQIENBMB4X
DTI1MTAxMjEzNTg1MVoXDTM1MTAxMjEzNTg1MVowKjELMAkGA1UEBhMCUlUxDzAN
BgNVBAoMBkdhbWVBUDEKMAgGA1UEAwwBKjCCASIwDQYJKoZIhvcNAQEBBQADggEP
ADCCAQoCggEBAKQROD/I2iPAGFrrO+iq9y5TcVFGooh1C8AKp1y5Rrwv7KHv3cBh
pL1Y7/1icxtr8Dg6oNDOjzV9u8YFs72EMjo1AwUgurtXD0tCktvt/bdX0Ff29BM/
B7GMUP2tUlnIoEyQdS0QVXqoVUrrs4qYAGk4dY88W2AIV5DHLH5/Ww8pgFxtcu5+
3fsxzBeZXzHMw1rOQxntrSzyr4tzHRGc+tI6bAjHPHE8ViLduTUlFq1l1NyUOHVh
rsWQy+e9AOE+ZXMGVDeWpmNPqL7o0+LDizE0JZEYndhUPDdsY30E1hMke+qNwWaI
psQ2+URGVC9eVbQusB1ceDFsAPqIxfM0/n0CAwEAAaMjMCEwHwYDVR0jBBgwFoAU
tnWbzarINqVyO1x8g4GC0hm2fXMwDQYJKoZIhvcNAQELBQADggEBAFh/jCD7JXi0
c7MkzO0GIQFu4SxNtsWCPSRpBXs4XV9VCVUr14Ja0RjnimQpyiv203RAVJNwUsrM
G7kjS7xpBvLKUIe2GTrqmlPAgIcGf1edqdmZWI/dGNSj1VE5Vzy7Ehfs+uWhNj9E
zvYZ2ypC1AIQeqqnr+SnzPolqqZM0Ei95Jk28DNpapu1kMJWhuM/2c9huLZrSrhW
dKuJHE8tZpcQ8CydU0D16qUhKCihi2hJDSCSbQFDtHAQHPx8TCYMts7IKzzrFuZZ
xNCggoLtZL8pvX+CQATnEIEEhdvRyi3hD9/mYh94LMfPxjiQOzMuOYH+y9iPnx5b
s1PL2QMvr5M=
-----END CERTIFICATE-----`

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name             string
		nodeID           uint
		setupRepo        func(*inmemory.NodeRepository)
		input            updateNodeInput
		setupFileManager func(*files.MockFileManager)
		expectedStatus   int
		expectError      bool
		validateResponse func(*testing.T, *domain.Node)
	}{
		{
			name:   "successful node update with all fields",
			nodeID: 1,
			setupRepo: func(repo *inmemory.NodeRepository) {
				now := time.Now()
				_ = repo.Save(context.Background(), &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "Old Node",
					OS:                  "linux",
					Location:            "US",
					Provider:            lo.ToPtr("OldProvider"),
					IPs:                 []string{"10.0.0.1"},
					WorkPath:            "/old/path",
					GdaemonHost:         "10.0.0.1",
					GdaemonPort:         12345,
					GdaemonAPIKey:       "old-api-key",
					GdaemonServerCert:   "certs/oldcert.crt",
					ClientCertificateID: 1,
					CreatedAt:           &now,
					UpdatedAt:           &now,
				})
			},
			input: updateNodeInput{
				Enabled:             lo.ToPtr(false),
				Name:                lo.ToPtr("Updated Node"),
				OS:                  lo.ToPtr("windows"),
				Location:            lo.ToPtr("EU"),
				Provider:            lo.ToPtr("NewProvider"),
				IP:                  []string{"192.168.1.1", "192.168.1.2"},
				RAM:                 lo.ToPtr("16GB"),
				CPU:                 lo.ToPtr("8 cores"),
				WorkPath:            lo.ToPtr("/new/path"),
				SteamcmdPath:        lo.ToPtr("/new/steamcmd"),
				GdaemonHost:         lo.ToPtr("192.168.1.1"),
				GdaemonPort:         lo.ToPtr(31717),
				GdaemonAPIKey:       lo.ToPtr("new-api-key"),
				GdaemonLogin:        lo.ToPtr("admin"),
				GdaemonPassword:     lo.ToPtr("password"),
				GdaemonServerCert:   lo.ToPtr(validCertPEM),
				ClientCertificateID: lo.ToPtr(uint(2)),
				PreferInstallMethod: lo.ToPtr("script"),
			},
			setupFileManager: func(fm *files.MockFileManager) {
				fm.WriteFunc = func(_ context.Context, _ string, _ []byte) error {
					return nil
				}
				fm.DeleteFunc = func(_ context.Context, _ string) error {
					return nil
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, node *domain.Node) {
				t.Helper()

				assert.False(t, node.Enabled)
				assert.Equal(t, "Updated Node", node.Name)
				assert.Equal(t, domain.NodeOSWindows, node.OS)
				assert.Equal(t, "EU", node.Location)
				assert.Equal(t, "NewProvider", *node.Provider)
				assert.Equal(t, domain.IPList{"192.168.1.1", "192.168.1.2"}, node.IPs)
				assert.Equal(t, "16GB", *node.RAM)
				assert.Equal(t, "8 cores", *node.CPU)
				assert.Equal(t, "/new/path", node.WorkPath)
				assert.Equal(t, "/new/steamcmd", *node.SteamcmdPath)
				assert.Equal(t, "192.168.1.1", node.GdaemonHost)
				assert.Equal(t, 31717, node.GdaemonPort)
				assert.Equal(t, "new-api-key", node.GdaemonAPIKey)
				assert.Equal(t, "admin", *node.GdaemonLogin)
				assert.Equal(t, "password", *node.GdaemonPassword)
				assert.Equal(t, uint(2), node.ClientCertificateID)
				assert.Equal(t, domain.NodePreferInstallMethodScript, node.PreferInstallMethod)
				assert.Contains(t, node.GdaemonServerCert, "certs/")
				assert.Contains(t, node.GdaemonServerCert, ".crt")
				assert.NotEqual(t, "certs/oldcert.crt", node.GdaemonServerCert)
			},
		},
		{
			name:   "successful partial update without certificate",
			nodeID: 2,
			setupRepo: func(repo *inmemory.NodeRepository) {
				now := time.Now()
				_ = repo.Save(context.Background(), &domain.Node{
					ID:                  2,
					Enabled:             true,
					Name:                "Test Node",
					OS:                  "linux",
					Location:            "US",
					IPs:                 []string{"10.0.0.2"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "10.0.0.2",
					GdaemonPort:         12345,
					GdaemonAPIKey:       "test-key",
					GdaemonServerCert:   "certs/test.crt",
					ClientCertificateID: 1,
					CreatedAt:           &now,
					UpdatedAt:           &now,
				})
			},
			input: updateNodeInput{
				Name:     lo.ToPtr("Partially Updated Node"),
				Location: lo.ToPtr("EU"),
			},
			setupFileManager: func(_ *files.MockFileManager) {},
			expectedStatus:   http.StatusOK,
			validateResponse: func(t *testing.T, node *domain.Node) {
				t.Helper()

				assert.Equal(t, "Partially Updated Node", node.Name)
				assert.Equal(t, "EU", node.Location)
				assert.Equal(t, domain.NodeOSLinux, node.OS)
				assert.Equal(t, domain.IPList{"10.0.0.2"}, node.IPs)
				assert.Equal(t, "certs/test.crt", node.GdaemonServerCert)
			},
		},
		{
			name:   "successful update with new certificate",
			nodeID: 3,
			setupRepo: func(repo *inmemory.NodeRepository) {
				now := time.Now()
				_ = repo.Save(context.Background(), &domain.Node{
					ID:                  3,
					Enabled:             true,
					Name:                "Cert Node",
					OS:                  "linux",
					Location:            "US",
					IPs:                 []string{"10.0.0.3"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "10.0.0.3",
					GdaemonPort:         12345,
					GdaemonAPIKey:       "cert-key",
					GdaemonServerCert:   "certs/oldcert.crt",
					ClientCertificateID: 1,
					CreatedAt:           &now,
					UpdatedAt:           &now,
				})
			},
			input: updateNodeInput{
				GdaemonServerCert: lo.ToPtr(validCertPEM),
			},
			setupFileManager: func(fm *files.MockFileManager) {
				fm.WriteFunc = func(_ context.Context, path string, data []byte) error {
					assert.Contains(t, path, "certs/")
					assert.Contains(t, path, ".crt")
					assert.Equal(t, validCertPEM, string(data))

					return nil
				}
				fm.DeleteFunc = func(_ context.Context, path string) error {
					assert.Equal(t, "certs/oldcert.crt", path)

					return nil
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, node *domain.Node) {
				t.Helper()

				assert.Contains(t, node.GdaemonServerCert, "certs/")
				assert.Contains(t, node.GdaemonServerCert, ".crt")
				assert.NotEqual(t, "certs/oldcert.crt", node.GdaemonServerCert)
			},
		},
		{
			name:   "node not found",
			nodeID: 999,
			setupRepo: func(_ *inmemory.NodeRepository) {
			},
			input:          updateNodeInput{},
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:   "invalid node id format",
			nodeID: 0,
			setupRepo: func(_ *inmemory.NodeRepository) {
			},
			input:          updateNodeInput{},
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:   "validation error - name too long",
			nodeID: 4,
			setupRepo: func(repo *inmemory.NodeRepository) {
				now := time.Now()
				_ = repo.Save(context.Background(), &domain.Node{
					ID:                  4,
					Enabled:             true,
					Name:                "Test",
					OS:                  "linux",
					Location:            "US",
					IPs:                 []string{"10.0.0.4"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "10.0.0.4",
					GdaemonPort:         12345,
					GdaemonAPIKey:       "test",
					GdaemonServerCert:   "certs/test.crt",
					ClientCertificateID: 1,
					CreatedAt:           &now,
					UpdatedAt:           &now,
				})
			},
			input: updateNodeInput{
				Name: lo.ToPtr(string(make([]byte, 200))),
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name:   "validation error - invalid IP",
			nodeID: 5,
			setupRepo: func(repo *inmemory.NodeRepository) {
				now := time.Now()
				_ = repo.Save(context.Background(), &domain.Node{
					ID:                  5,
					Enabled:             true,
					Name:                "Test",
					OS:                  "linux",
					Location:            "US",
					IPs:                 []string{"10.0.0.5"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "10.0.0.5",
					GdaemonPort:         12345,
					GdaemonAPIKey:       "test",
					GdaemonServerCert:   "certs/test.crt",
					ClientCertificateID: 1,
					CreatedAt:           &now,
					UpdatedAt:           &now,
				})
			},
			input: updateNodeInput{
				IP: []string{"invalid!!!"},
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name:   "successful update with valid hostname",
			nodeID: 10,
			setupRepo: func(repo *inmemory.NodeRepository) {
				now := time.Now()
				_ = repo.Save(context.Background(), &domain.Node{
					ID:                  10,
					Enabled:             true,
					Name:                "Test",
					OS:                  "linux",
					Location:            "US",
					IPs:                 []string{"10.0.0.10"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "10.0.0.10",
					GdaemonPort:         12345,
					GdaemonAPIKey:       "test",
					GdaemonServerCert:   "certs/test.crt",
					ClientCertificateID: 1,
					CreatedAt:           &now,
					UpdatedAt:           &now,
				})
			},
			input: updateNodeInput{
				IP: []string{"hldm.org", "gameap-daemon"},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:   "successful update with mixed IPs and hostnames",
			nodeID: 11,
			setupRepo: func(repo *inmemory.NodeRepository) {
				now := time.Now()
				_ = repo.Save(context.Background(), &domain.Node{
					ID:                  11,
					Enabled:             true,
					Name:                "Test",
					OS:                  "linux",
					Location:            "US",
					IPs:                 []string{"10.0.0.11"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "10.0.0.11",
					GdaemonPort:         12345,
					GdaemonAPIKey:       "test",
					GdaemonServerCert:   "certs/test.crt",
					ClientCertificateID: 1,
					CreatedAt:           &now,
					UpdatedAt:           &now,
				})
			},
			input: updateNodeInput{
				IP: []string{"192.168.1.1", "example.com", "game-server.example.com"},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:   "validation error - invalid OS",
			nodeID: 6,
			setupRepo: func(repo *inmemory.NodeRepository) {
				now := time.Now()
				_ = repo.Save(context.Background(), &domain.Node{
					ID:                  6,
					Enabled:             true,
					Name:                "Test",
					OS:                  "linux",
					Location:            "US",
					IPs:                 []string{"10.0.0.6"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "10.0.0.6",
					GdaemonPort:         12345,
					GdaemonAPIKey:       "test",
					GdaemonServerCert:   "certs/test.crt",
					ClientCertificateID: 1,
					CreatedAt:           &now,
					UpdatedAt:           &now,
				})
			},
			input: updateNodeInput{
				OS: lo.ToPtr("macos"),
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name:   "validation error - invalid port",
			nodeID: 7,
			setupRepo: func(repo *inmemory.NodeRepository) {
				now := time.Now()
				_ = repo.Save(context.Background(), &domain.Node{
					ID:                  7,
					Enabled:             true,
					Name:                "Test",
					OS:                  "linux",
					Location:            "US",
					IPs:                 []string{"10.0.0.7"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "10.0.0.7",
					GdaemonPort:         12345,
					GdaemonAPIKey:       "test",
					GdaemonServerCert:   "certs/test.crt",
					ClientCertificateID: 1,
					CreatedAt:           &now,
					UpdatedAt:           &now,
				})
			},
			input: updateNodeInput{
				GdaemonPort: lo.ToPtr(99999),
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name:   "file manager write error",
			nodeID: 8,
			setupRepo: func(repo *inmemory.NodeRepository) {
				now := time.Now()
				_ = repo.Save(context.Background(), &domain.Node{
					ID:                  8,
					Enabled:             true,
					Name:                "Test",
					OS:                  "linux",
					Location:            "US",
					IPs:                 []string{"10.0.0.8"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "10.0.0.8",
					GdaemonPort:         12345,
					GdaemonAPIKey:       "test",
					GdaemonServerCert:   "certs/test.crt",
					ClientCertificateID: 1,
					CreatedAt:           &now,
					UpdatedAt:           &now,
				})
			},
			input: updateNodeInput{
				GdaemonServerCert: lo.ToPtr(validCertPEM),
			},
			setupFileManager: func(fm *files.MockFileManager) {
				fm.WriteFunc = func(_ context.Context, _ string, _ []byte) error {
					return errors.New("write error")
				}
			},
			expectedStatus: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := inmemory.NewNodeRepository()
			fileManager := &files.MockFileManager{}
			responder := api.NewResponder()

			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}

			if tt.setupFileManager != nil {
				tt.setupFileManager(fileManager)
			}

			handler := NewHandler(repo, fileManager, responder)

			body, err := json.Marshal(tt.input)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPut, "/api/nodes/"+strconv.FormatUint(uint64(tt.nodeID), 10), bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"id": strconv.FormatUint(uint64(tt.nodeID), 10)})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectError {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
			}

			if tt.validateResponse != nil {
				var response nodeResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

				nodes, err := repo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.NotEmpty(t, nodes)

				var updatedNode *domain.Node
				for i := range nodes {
					if nodes[i].ID == tt.nodeID {
						updatedNode = &nodes[i]

						break
					}
				}
				require.NotNil(t, updatedNode)

				tt.validateResponse(t, updatedNode)
			}
		})
	}
}

func TestHandler_UpdatedAtTimestamp(t *testing.T) {
	repo := inmemory.NewNodeRepository()
	fileManager := &files.MockFileManager{}
	responder := api.NewResponder()

	oldTime := time.Now().Add(-1 * time.Hour)
	_ = repo.Save(context.Background(), &domain.Node{
		ID:                  1,
		Enabled:             true,
		Name:                "Test",
		OS:                  "linux",
		Location:            "US",
		IPs:                 []string{"10.0.0.1"},
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "10.0.0.1",
		GdaemonPort:         12345,
		GdaemonAPIKey:       "test",
		GdaemonServerCert:   "certs/test.crt",
		ClientCertificateID: 1,
		CreatedAt:           &oldTime,
		UpdatedAt:           &oldTime,
	})

	handler := NewHandler(repo, fileManager, responder)

	input := updateNodeInput{
		Name: lo.ToPtr("Updated Name"),
	}

	body, err := json.Marshal(input)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/nodes/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	nodes, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	assert.True(t, nodes[0].UpdatedAt.After(oldTime))
}

func TestHandler_CertificateFileCleanup(t *testing.T) {
	repo := inmemory.NewNodeRepository()
	deletedFiles := []string{}
	fileManager := &files.MockFileManager{
		WriteFunc: func(_ context.Context, _ string, _ []byte) error {
			return nil
		},
		DeleteFunc: func(_ context.Context, path string) error {
			deletedFiles = append(deletedFiles, path)

			return nil
		},
	}
	responder := api.NewResponder()

	now := time.Now()
	_ = repo.Save(context.Background(), &domain.Node{
		ID:                  1,
		Enabled:             true,
		Name:                "Test",
		OS:                  "linux",
		Location:            "US",
		IPs:                 []string{"10.0.0.1"},
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "10.0.0.1",
		GdaemonPort:         12345,
		GdaemonAPIKey:       "test",
		GdaemonServerCert:   "certs/oldcert.crt",
		ClientCertificateID: 1,
		CreatedAt:           &now,
		UpdatedAt:           &now,
	})

	handler := NewHandler(repo, fileManager, responder)

	input := updateNodeInput{
		GdaemonServerCert: lo.ToPtr(validCertPEM),
	}

	body, err := json.Marshal(input)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/nodes/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, deletedFiles, "certs/oldcert.crt")
}
