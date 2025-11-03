package postnode

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
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
		input            createDedicatedServerInput
		setupFileManager func(*files.MockFileManager)
		expectedStatus   int
		expectError      bool
		expectNodeID     bool
	}{
		{
			name: "successful dedicated server creation",
			input: createDedicatedServerInput{
				Name:                "Test Server",
				Location:            "US",
				IP:                  []string{"10.20.30.40"},
				OS:                  "linux",
				Enabled:             true,
				WorkPath:            "/srv/gameap",
				GdaemonHost:         "10.20.30.40",
				GdaemonPort:         12345,
				ClientCertificateID: 1,
				GdaemonServerCert:   validCertPEM,
			},
			setupFileManager: func(fm *files.MockFileManager) {
				fm.WriteFunc = func(_ context.Context, _ string, _ []byte) error {
					return nil
				}
			},
			expectedStatus: http.StatusCreated,
			expectNodeID:   true,
		},
		{
			name: "successful creation with optional fields",
			input: createDedicatedServerInput{
				Name:                "Test Server",
				Description:         stringPtr("Test description"),
				Location:            "Montenegro",
				IP:                  []string{"172.18.0.5", "10.0.0.1"},
				OS:                  "windows",
				Enabled:             false,
				Provider:            stringPtr("AWS"),
				WorkPath:            "/srv/gameap",
				SteamcmdPath:        stringPtr("/srv/gameap/steamcmd"),
				GdaemonHost:         "172.18.0.5",
				GdaemonPort:         31717,
				ClientCertificateID: 2,
				GdaemonServerCert:   validCertPEM,
			},
			setupFileManager: func(fm *files.MockFileManager) {
				fm.WriteFunc = func(_ context.Context, _ string, _ []byte) error {
					return nil
				}
			},
			expectedStatus: http.StatusCreated,
			expectNodeID:   true,
		},
		{
			name: "name is required",
			input: createDedicatedServerInput{
				Location:            "US",
				IP:                  []string{"10.20.30.40"},
				OS:                  "linux",
				WorkPath:            "/srv/gameap",
				GdaemonHost:         "10.20.30.40",
				GdaemonPort:         12345,
				ClientCertificateID: 1,
				GdaemonServerCert:   validCertPEM,
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name: "location is required",
			input: createDedicatedServerInput{
				Name:                "Test",
				IP:                  []string{"10.20.30.40"},
				OS:                  "linux",
				WorkPath:            "/srv/gameap",
				GdaemonHost:         "10.20.30.40",
				GdaemonPort:         12345,
				ClientCertificateID: 1,
				GdaemonServerCert:   validCertPEM,
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name: "IP is required",
			input: createDedicatedServerInput{
				Name:                "Test",
				Location:            "US",
				IP:                  []string{},
				OS:                  "linux",
				WorkPath:            "/srv/gameap",
				GdaemonHost:         "10.20.30.40",
				GdaemonPort:         12345,
				ClientCertificateID: 1,
				GdaemonServerCert:   validCertPEM,
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name: "invalid IP address",
			input: createDedicatedServerInput{
				Name:                "Test",
				Location:            "US",
				IP:                  []string{"invalid-ip"},
				OS:                  "linux",
				WorkPath:            "/srv/gameap",
				GdaemonHost:         "10.20.30.40",
				GdaemonPort:         12345,
				ClientCertificateID: 1,
				GdaemonServerCert:   validCertPEM,
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name: "OS is required",
			input: createDedicatedServerInput{
				Name:                "Test",
				Location:            "US",
				IP:                  []string{"10.20.30.40"},
				WorkPath:            "/srv/gameap",
				GdaemonHost:         "10.20.30.40",
				GdaemonPort:         12345,
				ClientCertificateID: 1,
				GdaemonServerCert:   validCertPEM,
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name: "invalid OS value",
			input: createDedicatedServerInput{
				Name:                "Test",
				Location:            "US",
				IP:                  []string{"10.20.30.40"},
				OS:                  "macos",
				WorkPath:            "/srv/gameap",
				GdaemonHost:         "10.20.30.40",
				GdaemonPort:         12345,
				ClientCertificateID: 1,
				GdaemonServerCert:   validCertPEM,
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name: "work_path is required",
			input: createDedicatedServerInput{
				Name:                "Test",
				Location:            "US",
				IP:                  []string{"10.20.30.40"},
				OS:                  "linux",
				GdaemonHost:         "10.20.30.40",
				GdaemonPort:         12345,
				ClientCertificateID: 1,
				GdaemonServerCert:   validCertPEM,
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name: "gdaemon_host is required",
			input: createDedicatedServerInput{
				Name:                "Test",
				Location:            "US",
				IP:                  []string{"10.20.30.40"},
				OS:                  "linux",
				WorkPath:            "/srv/gameap",
				GdaemonPort:         12345,
				ClientCertificateID: 1,
				GdaemonServerCert:   validCertPEM,
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name: "gdaemon_port is required",
			input: createDedicatedServerInput{
				Name:                "Test",
				Location:            "US",
				IP:                  []string{"10.20.30.40"},
				OS:                  "linux",
				WorkPath:            "/srv/gameap",
				GdaemonHost:         "10.20.30.40",
				ClientCertificateID: 1,
				GdaemonServerCert:   validCertPEM,
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name: "invalid gdaemon_port",
			input: createDedicatedServerInput{
				Name:                "Test",
				Location:            "US",
				IP:                  []string{"10.20.30.40"},
				OS:                  "linux",
				WorkPath:            "/srv/gameap",
				GdaemonHost:         "10.20.30.40",
				GdaemonPort:         99999,
				ClientCertificateID: 1,
				GdaemonServerCert:   validCertPEM,
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name: "client_certificate_id is required",
			input: createDedicatedServerInput{
				Name:              "Test",
				Location:          "US",
				IP:                []string{"10.20.30.40"},
				OS:                "linux",
				WorkPath:          "/srv/gameap",
				GdaemonHost:       "10.20.30.40",
				GdaemonPort:       12345,
				GdaemonServerCert: validCertPEM,
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name: "gdaemon_server_cert is required",
			input: createDedicatedServerInput{
				Name:                "Test",
				Location:            "US",
				IP:                  []string{"10.20.30.40"},
				OS:                  "linux",
				WorkPath:            "/srv/gameap",
				GdaemonHost:         "10.20.30.40",
				GdaemonPort:         12345,
				ClientCertificateID: 1,
			},
			expectedStatus: http.StatusInternalServerError,
			expectError:    true,
		},
		{
			name: "file manager write error",
			input: createDedicatedServerInput{
				Name:                "Test",
				Location:            "US",
				IP:                  []string{"10.20.30.40"},
				OS:                  "linux",
				WorkPath:            "/srv/gameap",
				GdaemonHost:         "10.20.30.40",
				GdaemonPort:         12345,
				ClientCertificateID: 1,
				GdaemonServerCert:   validCertPEM,
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

			if tt.setupFileManager != nil {
				tt.setupFileManager(fileManager)
			}

			handler := NewHandler(repo, fileManager, responder)

			body, err := json.Marshal(tt.input)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/dedicated_servers", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(context.Background())
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectError {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
			}

			if tt.expectNodeID {
				var response dedicatedServerResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.NotZero(t, response.ID)
				assert.Equal(t, "success", response.Message)
				assert.Equal(t, response.ID, response.Result)
			}
		})
	}
}

func TestHandler_NodeStoredCorrectly(t *testing.T) {
	repo := inmemory.NewNodeRepository()
	fileManager := &files.MockFileManager{
		WriteFunc: func(_ context.Context, _ string, _ []byte) error {
			return nil
		},
	}
	responder := api.NewResponder()
	handler := NewHandler(repo, fileManager, responder)

	input := createDedicatedServerInput{
		Name:                "Test Server",
		Description:         stringPtr("Test description"),
		Location:            "Montenegro",
		IP:                  []string{"172.18.0.5"},
		OS:                  "linux",
		Enabled:             true,
		Provider:            stringPtr("Unknown"),
		WorkPath:            "/srv/gameap",
		SteamcmdPath:        stringPtr("/srv/gameap/steamcmd"),
		GdaemonHost:         "10.20.30.40",
		GdaemonPort:         12345,
		ClientCertificateID: 1,
		GdaemonServerCert:   validCertPEM,
	}

	body, err := json.Marshal(input)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/dedicated_servers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.Background())
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var response dedicatedServerResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	nodes, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	node := nodes[0]
	assert.Equal(t, response.ID, node.ID)
	assert.Equal(t, "Test Server", node.Name)
	assert.Equal(t, "Unknown", *node.Provider)
	assert.Equal(t, "Montenegro", node.Location)
	assert.Equal(t, domain.IPList{"172.18.0.5"}, node.IPs)
	assert.Equal(t, domain.NodeOSLinux, node.OS)
	assert.True(t, node.Enabled)
	assert.Equal(t, "/srv/gameap", node.WorkPath)
	assert.Equal(t, "/srv/gameap/steamcmd", *node.SteamcmdPath)
	assert.Equal(t, "10.20.30.40", node.GdaemonHost)
	assert.Equal(t, 12345, node.GdaemonPort)
	assert.Equal(t, uint(1), node.ClientCertificateID)
	assert.NotEmpty(t, node.GdaemonAPIKey)
	assert.Contains(t, node.GdaemonServerCert, "certs/")
	assert.Contains(t, node.GdaemonServerCert, ".crt")
	assert.NotNil(t, node.CreatedAt)
	assert.NotNil(t, node.UpdatedAt)
}

func TestHandler_CertificateSavedToFile(t *testing.T) {
	repo := inmemory.NewNodeRepository()
	var savedPath string
	var savedContent []byte
	fileManager := &files.MockFileManager{
		WriteFunc: func(_ context.Context, path string, data []byte) error {
			savedPath = path
			savedContent = data

			return nil
		},
	}
	responder := api.NewResponder()
	handler := NewHandler(repo, fileManager, responder)

	input := createDedicatedServerInput{
		Name:                "Test",
		Location:            "US",
		IP:                  []string{"10.20.30.40"},
		OS:                  "linux",
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "10.20.30.40",
		GdaemonPort:         12345,
		ClientCertificateID: 1,
		GdaemonServerCert:   validCertPEM,
	}

	body, err := json.Marshal(input)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/dedicated_servers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.Background())
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	assert.Contains(t, savedPath, "certs/")
	assert.Contains(t, savedPath, ".crt")
	assert.Equal(t, validCertPEM, string(savedContent))

	nodes, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, savedPath, nodes[0].GdaemonServerCert)
}

func TestHandler_APIKeyGenerated(t *testing.T) {
	repo := inmemory.NewNodeRepository()
	fileManager := &files.MockFileManager{
		WriteFunc: func(_ context.Context, _ string, _ []byte) error {
			return nil
		},
	}
	responder := api.NewResponder()
	handler := NewHandler(repo, fileManager, responder)

	input := createDedicatedServerInput{
		Name:                "Test",
		Location:            "US",
		IP:                  []string{"10.20.30.40"},
		OS:                  "linux",
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "10.20.30.40",
		GdaemonPort:         12345,
		ClientCertificateID: 1,
		GdaemonServerCert:   validCertPEM,
	}

	body, err := json.Marshal(input)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/dedicated_servers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.Background())
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	nodes, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	assert.Len(t, nodes[0].GdaemonAPIKey, apiKeyLength)
	assert.NotEmpty(t, nodes[0].GdaemonAPIKey)
}

func stringPtr(s string) *string {
	return &s
}
