package createnode

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	daemonbase "github.com/gameap/gameap/internal/api/daemon/base"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/certificates"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCSR = `-----BEGIN CERTIFICATE REQUEST-----
MIICkzCCAXsCAQAwTjELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUx
FTATBgNVBAoMDEdhbWVBUCBUZXN0czETMBEGA1UEAwwKZ2FtZWFwLmNvbTCCASIw
DQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBALViwjNl9v5z30CryJdfy04trmZX
eq2UpWcVJwupjJEyc8yJkdsSbVkz0dVPym81oxhHAxLnEccMoSkyEhKyNA2+RfRw
174fS/0yW1pUlJedvBE5Bcj9NC8LValVpv6l1o5+Bw+MmQ1uydpcME33PMVQMSG+
FemjlXvtb9oQnz/4EuA4WMWJVqcNnUh30P/tSlFtgmHDbjWpc1wObl/2zkVjD24Q
OiJss5QXCvVNu2qiMyX2jSLqJVPPdjRicJABdKf6G0roPzC4ZOdMoBOkd0r5sAUW
tNQRY2BqOt975hpejdDjJZXOZ4o7xtm90gML5hAbg+C20UqIgLD4QEQlsakCAwEA
AaAAMA0GCSqGSIb3DQEBCwUAA4IBAQCf9Gh6MtGyNnwHyih3DIA8S0aseGQno5kt
pubT9g2+7Lh8MDhfDZdyrUzynmEr1/33eqdmmsub6AsskQDg5VBbdIrbPt+BN1IF
o0iXjhhO/9YXR8w7asHiF6gzhVqYrNo8S18rEcipbJPTFt3g640WM3231V62t/p8
Mo5f6PEOEA5cQgYrGBoIKi2eiVOdfVfviJo9oOEFXHBy7UAxII1fYcjPVIaaSQii
ul9sZUsckzpylCoJow/b6hyC+O9F2CE7JZEsK+67WpP3ztN0c9iioQJOwZAEIJA9
szwg1cQQRyOPTHIZO4rqdqK0hYF+H6lKfBrE2Rt4Dsarc2Vh2vY6
-----END CERTIFICATE REQUEST-----`

func TestHandler_ServeHTTP_Success(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	nodesRepo := inmemory.NewNodeRepository()
	clientCertsRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()
	certsSvc := certificates.NewService(fileManager)
	responder := api.NewResponder()

	handler := NewHandler(cacheInstance, nodesRepo, clientCertsRepo, certsSvc, responder)

	err := cacheInstance.Set(context.Background(), daemonbase.AutoCreateTokenCacheKey, "test-token", cache.WithExpiration(300*time.Second))
	require.NoError(t, err)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("gdaemon_server_cert", "server.csr")
	require.NoError(t, err)
	_, err = part.Write([]byte(testCSR))
	require.NoError(t, err)

	err = writer.WriteField("ip[]", "192.168.1.100")
	require.NoError(t, err)
	err = writer.WriteField("location", "Test Location")
	require.NoError(t, err)
	err = writer.WriteField("gdaemon_host", "gameap.example.com")
	require.NoError(t, err)
	err = writer.WriteField("gdaemon_port", "31717")
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/gdaemon/create/test-token", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = mux.SetURLVars(req, map[string]string{"token": "test-token"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))

	responseBody := w.Body.String()
	assert.Contains(t, responseBody, "Success")
	assert.Contains(t, responseBody, "BEGIN CERTIFICATE")

	val, err := cacheInstance.Get(context.Background(), daemonbase.AutoCreateTokenCacheKey)
	assert.True(t, err != nil || val == nil, "token should be deleted from cache")
}

func TestHandler_ServeHTTP_InvalidToken(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	nodesRepo := inmemory.NewNodeRepository()
	clientCertsRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()
	certsSvc := certificates.NewService(fileManager)
	responder := api.NewResponder()

	handler := NewHandler(cacheInstance, nodesRepo, clientCertsRepo, certsSvc, responder)

	err := cacheInstance.Set(context.Background(), daemonbase.AutoCreateTokenCacheKey, "valid-token", cache.WithExpiration(300*time.Second))
	require.NoError(t, err)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("gdaemon_server_cert", "server.csr")
	require.NoError(t, err)
	_, err = part.Write([]byte(testCSR))
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/gdaemon/create/wrong-token", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = mux.SetURLVars(req, map[string]string{"token": "wrong-token"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestHandler_ServeHTTP_TokenNotFound(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	nodesRepo := inmemory.NewNodeRepository()
	clientCertsRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()
	certsSvc := certificates.NewService(fileManager)
	responder := api.NewResponder()

	handler := NewHandler(cacheInstance, nodesRepo, clientCertsRepo, certsSvc, responder)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("gdaemon_server_cert", "server.csr")
	require.NoError(t, err)
	_, err = part.Write([]byte(testCSR))
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/gdaemon/create/nonexistent", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = mux.SetURLVars(req, map[string]string{"token": "nonexistent"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestHandler_ServeHTTP_MissingCertificate(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	nodesRepo := inmemory.NewNodeRepository()
	clientCertsRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()
	certsSvc := certificates.NewService(fileManager)
	responder := api.NewResponder()

	handler := NewHandler(cacheInstance, nodesRepo, clientCertsRepo, certsSvc, responder)

	err := cacheInstance.Set(context.Background(), daemonbase.AutoCreateTokenCacheKey, "test-token", cache.WithExpiration(300*time.Second))
	require.NoError(t, err)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	err = writer.WriteField("ip[]", "192.168.1.100")
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/gdaemon/create/test-token", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = mux.SetURLVars(req, map[string]string{"token": "test-token"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestHandler_ServeHTTP_InvalidPort(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	nodesRepo := inmemory.NewNodeRepository()
	clientCertsRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()
	certsSvc := certificates.NewService(fileManager)
	responder := api.NewResponder()

	handler := NewHandler(cacheInstance, nodesRepo, clientCertsRepo, certsSvc, responder)

	err := cacheInstance.Set(context.Background(), daemonbase.AutoCreateTokenCacheKey, "test-token", cache.WithExpiration(300*time.Second))
	require.NoError(t, err)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("gdaemon_server_cert", "server.csr")
	require.NoError(t, err)
	_, err = part.Write([]byte(testCSR))
	require.NoError(t, err)

	err = writer.WriteField("gdaemon_port", "99999")
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/gdaemon/create/test-token", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = mux.SetURLVars(req, map[string]string{"token": "test-token"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestHandler_ServeHTTP_DefaultValues(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	nodesRepo := inmemory.NewNodeRepository()
	clientCertsRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()
	certsSvc := certificates.NewService(fileManager)
	responder := api.NewResponder()

	handler := NewHandler(cacheInstance, nodesRepo, clientCertsRepo, certsSvc, responder)

	err := cacheInstance.Set(context.Background(), daemonbase.AutoCreateTokenCacheKey, "test-token", cache.WithExpiration(300*time.Second))
	require.NoError(t, err)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("gdaemon_server_cert", "server.csr")
	require.NoError(t, err)
	_, err = part.Write([]byte(testCSR))
	require.NoError(t, err)

	err = writer.WriteField("ip[]", "10.0.0.1")
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/gdaemon/create/test-token", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = mux.SetURLVars(req, map[string]string{"token": "test-token"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	nodes, err := nodesRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	node := nodes[0]
	assert.Equal(t, "Unknown", node.Location)
	assert.Equal(t, "10.0.0.1", node.GdaemonHost)
	assert.Equal(t, defaultPort, node.GdaemonPort)
	assert.True(t, node.Enabled)
	assert.NotEmpty(t, node.GdaemonAPIKey)
	assert.Equal(t, certificates.RootCACert, node.GdaemonServerCert)
}

func TestHandler_ServeHTTP_WithAllFields(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	nodesRepo := inmemory.NewNodeRepository()
	clientCertsRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()
	certsSvc := certificates.NewService(fileManager)
	responder := api.NewResponder()

	handler := NewHandler(cacheInstance, nodesRepo, clientCertsRepo, certsSvc, responder)

	err := cacheInstance.Set(context.Background(), daemonbase.AutoCreateTokenCacheKey, "test-token", cache.WithExpiration(300*time.Second))
	require.NoError(t, err)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("gdaemon_server_cert", "server.csr")
	require.NoError(t, err)
	_, err = part.Write([]byte(testCSR))
	require.NoError(t, err)

	err = writer.WriteField("ip[]", "172.16.0.1")
	require.NoError(t, err)
	err = writer.WriteField("location", "Custom Location")
	require.NoError(t, err)
	err = writer.WriteField("gdaemon_host", "custom.example.com")
	require.NoError(t, err)
	err = writer.WriteField("gdaemon_port", "9000")
	require.NoError(t, err)
	err = writer.WriteField("provider", "AWS")
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/gdaemon/create/test-token", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = mux.SetURLVars(req, map[string]string{"token": "test-token"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	nodes, err := nodesRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	node := nodes[0]
	assert.Equal(t, "Custom Location", node.Location)
	assert.Equal(t, "custom.example.com", node.GdaemonHost)
	assert.Equal(t, 9000, node.GdaemonPort)
	assert.Equal(t, domain.IPList{"172.16.0.1"}, node.IPs)
	require.NotNil(t, node.Provider)
	assert.Equal(t, "AWS", *node.Provider)
}

func TestHandler_ResponseFormat(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	nodesRepo := inmemory.NewNodeRepository()
	clientCertsRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()
	certsSvc := certificates.NewService(fileManager)
	responder := api.NewResponder()

	handler := NewHandler(cacheInstance, nodesRepo, clientCertsRepo, certsSvc, responder)

	err := cacheInstance.Set(context.Background(), daemonbase.AutoCreateTokenCacheKey, "test-token", cache.WithExpiration(300*time.Second))
	require.NoError(t, err)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("gdaemon_server_cert", "server.csr")
	require.NoError(t, err)
	_, err = part.Write([]byte(testCSR))
	require.NoError(t, err)

	err = writer.WriteField("ip[]", "192.168.1.1")
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/gdaemon/create/test-token", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = mux.SetURLVars(req, map[string]string{"token": "test-token"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	responseBody := w.Body.String()

	lines := strings.Split(responseBody, "\n")
	require.GreaterOrEqual(t, len(lines), 3)

	firstLine := lines[0]
	assert.True(t, strings.HasPrefix(firstLine, "Success "), "first line should start with 'Success'")

	parts := strings.Fields(firstLine)
	require.Len(t, parts, 3, "first line should contain: Success <id> <api_key>")

	assert.Equal(t, "Success", parts[0])

	nodeID := parts[1]
	assert.NotEmpty(t, nodeID)

	apiKey := parts[2]
	assert.Len(t, apiKey, apiKeyLength, "API key should have correct length")

	assert.Contains(t, responseBody, "BEGIN CERTIFICATE", "response should contain root certificate")
	assert.Contains(t, responseBody, "END CERTIFICATE", "response should contain end certificate marker")
}

func TestBuildCreateResponse(t *testing.T) {
	nodeID := uint(42)
	apiKey := "test-api-key-1234567890"
	rootCert := "-----BEGIN CERTIFICATE-----\nROOT\n-----END CERTIFICATE-----"
	signedCert := "-----BEGIN CERTIFICATE-----\nSIGNED\n-----END CERTIFICATE-----"

	response := buildCreateResponse(nodeID, apiKey, rootCert, signedCert)

	expectedFormat := fmt.Sprintf("Success %d %s\n%s\n\n%s", nodeID, apiKey, rootCert, signedCert)
	assert.Equal(t, expectedFormat, response)

	assert.Contains(t, response, "Success 42")
	assert.Contains(t, response, apiKey)
	assert.Contains(t, response, rootCert)
	assert.Contains(t, response, signedCert)
}

func TestNodeInput_Validate(t *testing.T) {
	tests := []struct {
		name      string
		input     *nodeInput
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid input with all fields",
			input: &nodeInput{
				IP:                []string{"192.168.1.1"},
				GdaemonHost:       "example.com",
				GdaemonPort:       "31717",
				Location:          "Test",
				GdaemonServerCert: []*multipart.FileHeader{{Size: 100}},
			},
			wantError: false,
		},
		{
			name: "valid input with minimal fields",
			input: &nodeInput{
				GdaemonServerCert: []*multipart.FileHeader{{Size: 100}},
			},
			wantError: false,
		},
		{
			name: "missing certificate file",
			input: &nodeInput{
				IP:                []string{"192.168.1.1"},
				GdaemonServerCert: []*multipart.FileHeader{},
			},
			wantError: true,
			errorMsg:  "gdaemon_server_cert file is required",
		},
		{
			name: "empty certificate file",
			input: &nodeInput{
				GdaemonServerCert: []*multipart.FileHeader{{Size: 0}},
			},
			wantError: true,
			errorMsg:  "invalid gdaemon_server_cert file",
		},
		{
			name: "invalid port - too high",
			input: &nodeInput{
				GdaemonPort:       "99999",
				GdaemonServerCert: []*multipart.FileHeader{{Size: 100}},
			},
			wantError: true,
			errorMsg:  "gdaemon_port must be between 1 and 65535",
		},
		{
			name: "invalid port - too low",
			input: &nodeInput{
				GdaemonPort:       "0",
				GdaemonServerCert: []*multipart.FileHeader{{Size: 100}},
			},
			wantError: true,
			errorMsg:  "gdaemon_port must be between 1 and 65535",
		},
		{
			name: "invalid port - not a number",
			input: &nodeInput{
				GdaemonPort:       "invalid",
				GdaemonServerCert: []*multipart.FileHeader{{Size: 100}},
			},
			wantError: true,
			errorMsg:  "gdaemon_port must be between 1 and 65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNodeInput_ToDomain(t *testing.T) {
	tests := []struct {
		name       string
		input      *nodeInput
		apiKey     string
		serverCert string
		checkNode  func(*testing.T, *domain.Node)
	}{
		{
			name: "all_fields_provided",
			input: &nodeInput{
				Name:                "US East",
				IP:                  []string{"10.0.0.1"},
				GdaemonHost:         "gameap.example.com",
				GdaemonPort:         "9000",
				Location:            "US East",
				Provider:            "AWS",
				OS:                  "linux",
				WorkPath:            "/custom/path",
				SteamcmdPath:        "/usr/games/steamcmd",
				PreferInstallMethod: "steam",
				ScriptInstall:       "install.sh",
				ScriptReinstall:     "reinstall.sh",
				ScriptUpdate:        "update.sh",
				ScriptStart:         "start.sh",
				ScriptPause:         "pause.sh",
				ScriptUnpause:       "unpause.sh",
				ScriptStop:          "stop.sh",
				ScriptKill:          "kill.sh",
				ScriptRestart:       "restart.sh",
				ScriptStatus:        "status.sh",
				ScriptStats:         "stats.sh",
				ScriptGetConsole:    "console.sh",
				ScriptSendCommand:   "command.sh",
				ScriptDelete:        "delete.sh",
			},
			apiKey:     "test-api-key",
			serverCert: "test-cert-path",
			checkNode: func(t *testing.T, node *domain.Node) {
				t.Helper()

				assert.Equal(t, "US East", node.Location)
				assert.Equal(t, "US East", node.Name)
				assert.Equal(t, "gameap.example.com", node.GdaemonHost)
				assert.Equal(t, 9000, node.GdaemonPort)
				assert.Equal(t, domain.IPList{"10.0.0.1"}, node.IPs)
				assert.Equal(t, "test-api-key", node.GdaemonAPIKey)
				assert.Equal(t, "test-cert-path", node.GdaemonServerCert)
				require.NotNil(t, node.Provider)
				assert.Equal(t, "AWS", *node.Provider)
				assert.True(t, node.Enabled)
				assert.Equal(t, domain.NodeOSLinux, node.OS)
				assert.Equal(t, "/custom/path", node.WorkPath)
				require.NotNil(t, node.SteamcmdPath)
				assert.Equal(t, "/usr/games/steamcmd", *node.SteamcmdPath)
				assert.Equal(t, domain.NodePreferInstallMethod("steam"), node.PreferInstallMethod)
				require.NotNil(t, node.ScriptInstall)
				assert.Equal(t, "install.sh", *node.ScriptInstall)
				require.NotNil(t, node.ScriptReinstall)
				assert.Equal(t, "reinstall.sh", *node.ScriptReinstall)
				require.NotNil(t, node.ScriptUpdate)
				assert.Equal(t, "update.sh", *node.ScriptUpdate)
				require.NotNil(t, node.ScriptStart)
				assert.Equal(t, "start.sh", *node.ScriptStart)
				require.NotNil(t, node.ScriptPause)
				assert.Equal(t, "pause.sh", *node.ScriptPause)
				require.NotNil(t, node.ScriptUnpause)
				assert.Equal(t, "unpause.sh", *node.ScriptUnpause)
				require.NotNil(t, node.ScriptStop)
				assert.Equal(t, "stop.sh", *node.ScriptStop)
				require.NotNil(t, node.ScriptKill)
				assert.Equal(t, "kill.sh", *node.ScriptKill)
				require.NotNil(t, node.ScriptRestart)
				assert.Equal(t, "restart.sh", *node.ScriptRestart)
				require.NotNil(t, node.ScriptStatus)
				assert.Equal(t, "status.sh", *node.ScriptStatus)
				require.NotNil(t, node.ScriptStats)
				assert.Equal(t, "stats.sh", *node.ScriptStats)
				require.NotNil(t, node.ScriptGetConsole)
				assert.Equal(t, "console.sh", *node.ScriptGetConsole)
				require.NotNil(t, node.ScriptSendCommand)
				assert.Equal(t, "command.sh", *node.ScriptSendCommand)
				require.NotNil(t, node.ScriptDelete)
				assert.Equal(t, "delete.sh", *node.ScriptDelete)
			},
		},
		{
			name: "minimal_fields_with_defaults",
			input: &nodeInput{
				IP: []string{"192.168.1.1"},
			},
			apiKey:     "minimal-key",
			serverCert: "minimal-cert",
			checkNode: func(t *testing.T, node *domain.Node) {
				t.Helper()

				assert.Equal(t, "Unknown", node.Location)
				assert.NotEmpty(t, node.Name)
				assert.Equal(t, "192.168.1.1", node.GdaemonHost)
				assert.Equal(t, defaultPort, node.GdaemonPort)
				assert.Equal(t, domain.IPList{"192.168.1.1"}, node.IPs)
				require.NotNil(t, node.Provider)
				assert.Equal(t, "Unknown", *node.Provider)
				assert.Equal(t, domain.NodeOSOther, node.OS)
				assert.Equal(t, "/srv/gameap", node.WorkPath)
				assert.Nil(t, node.SteamcmdPath)
				assert.Equal(t, domain.NodePreferInstallMethodAuto, node.PreferInstallMethod)
				assert.Nil(t, node.ScriptInstall)
				assert.Nil(t, node.ScriptReinstall)
				assert.Nil(t, node.ScriptUpdate)
				assert.Nil(t, node.ScriptStart)
				assert.Nil(t, node.ScriptPause)
				assert.Nil(t, node.ScriptUnpause)
				assert.Nil(t, node.ScriptStop)
				assert.Nil(t, node.ScriptKill)
				assert.Nil(t, node.ScriptRestart)
				assert.Nil(t, node.ScriptStatus)
				assert.Nil(t, node.ScriptStats)
				assert.Nil(t, node.ScriptGetConsole)
				assert.Nil(t, node.ScriptSendCommand)
				assert.Nil(t, node.ScriptDelete)
			},
		},
		{
			name: "empty_IP_defaults_to_empty_IPs_array",
			input: &nodeInput{
				GdaemonHost: "example.com",
				Location:    "Test Location",
			},
			apiKey:     "test-key",
			serverCert: "test-cert",
			checkNode: func(t *testing.T, node *domain.Node) {
				t.Helper()

				assert.Equal(t, "Test Location", node.Location)
				assert.Equal(t, "example.com", node.GdaemonHost)
				assert.Nil(t, node.IPs)
			},
		},
		{
			name: "empty_gdaemon_host_uses_IP",
			input: &nodeInput{
				IP:       []string{"203.0.113.1"},
				Location: "Test",
			},
			apiKey:     "test-key",
			serverCert: "test-cert",
			checkNode: func(t *testing.T, node *domain.Node) {
				t.Helper()

				assert.Equal(t, "203.0.113.1", node.GdaemonHost)
			},
		},
		{
			name: "OS_parsing_linux",
			input: &nodeInput{
				IP: []string{"192.168.1.1"},
				OS: "ubuntu",
			},
			apiKey:     "test-key",
			serverCert: "test-cert",
			checkNode: func(t *testing.T, node *domain.Node) {
				t.Helper()

				assert.Equal(t, domain.NodeOSLinux, node.OS)
			},
		},
		{
			name: "OS_parsing_windows",
			input: &nodeInput{
				IP: []string{"192.168.1.1"},
				OS: "windows",
			},
			apiKey:     "test-key",
			serverCert: "test-cert",
			checkNode: func(t *testing.T, node *domain.Node) {
				t.Helper()

				assert.Equal(t, domain.NodeOSWindows, node.OS)
			},
		},
		{
			name: "custom_work_path",
			input: &nodeInput{
				IP:       []string{"192.168.1.1"},
				WorkPath: "/opt/gameap",
			},
			apiKey:     "test-key",
			serverCert: "test-cert",
			checkNode: func(t *testing.T, node *domain.Node) {
				t.Helper()

				assert.Equal(t, "/opt/gameap", node.WorkPath)
			},
		},
		{
			name: "steamcmd_path_provided",
			input: &nodeInput{
				IP:           []string{"192.168.1.1"},
				SteamcmdPath: "/usr/local/bin/steamcmd",
			},
			apiKey:     "test-key",
			serverCert: "test-cert",
			checkNode: func(t *testing.T, node *domain.Node) {
				t.Helper()

				require.NotNil(t, node.SteamcmdPath)
				assert.Equal(t, "/usr/local/bin/steamcmd", *node.SteamcmdPath)
			},
		},
		{
			name: "prefer_install_method_copy",
			input: &nodeInput{
				IP:                  []string{"192.168.1.1"},
				PreferInstallMethod: "copy",
			},
			apiKey:     "test-key",
			serverCert: "test-cert",
			checkNode: func(t *testing.T, node *domain.Node) {
				t.Helper()

				assert.Equal(t, domain.NodePreferInstallMethod("copy"), node.PreferInstallMethod)
			},
		},
		{
			name: "scripts_provided",
			input: &nodeInput{
				IP:                []string{"192.168.1.1"},
				ScriptStart:       "systemctl start game",
				ScriptStop:        "systemctl stop game",
				ScriptRestart:     "systemctl restart game",
				ScriptStatus:      "systemctl status game",
				ScriptGetConsole:  "tail -f /var/log/game.log",
				ScriptSendCommand: "echo '{command}' > /var/run/game.fifo",
			},
			apiKey:     "test-key",
			serverCert: "test-cert",
			checkNode: func(t *testing.T, node *domain.Node) {
				t.Helper()

				require.NotNil(t, node.ScriptStart)
				assert.Equal(t, "systemctl start game", *node.ScriptStart)
				require.NotNil(t, node.ScriptStop)
				assert.Equal(t, "systemctl stop game", *node.ScriptStop)
				require.NotNil(t, node.ScriptRestart)
				assert.Equal(t, "systemctl restart game", *node.ScriptRestart)
				require.NotNil(t, node.ScriptStatus)
				assert.Equal(t, "systemctl status game", *node.ScriptStatus)
				require.NotNil(t, node.ScriptGetConsole)
				assert.Equal(t, "tail -f /var/log/game.log", *node.ScriptGetConsole)
				require.NotNil(t, node.ScriptSendCommand)
				assert.Equal(t, "echo '{command}' > /var/run/game.fifo", *node.ScriptSendCommand)

				assert.Nil(t, node.ScriptInstall)
				assert.Nil(t, node.ScriptReinstall)
				assert.Nil(t, node.ScriptUpdate)
				assert.Nil(t, node.ScriptPause)
				assert.Nil(t, node.ScriptUnpause)
				assert.Nil(t, node.ScriptKill)
				assert.Nil(t, node.ScriptStats)
				assert.Nil(t, node.ScriptDelete)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := tt.input.ToDomain(tt.apiKey, tt.serverCert)

			require.NotNil(t, node)
			tt.checkNode(t, node)
		})
	}
}
