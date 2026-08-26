package setupkey

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/certificates"
	"github.com/gameap/gameap/internal/enrollment"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupEnrollmentService(t *testing.T) (*enrollment.Service, cache.Cache) {
	t.Helper()

	cacheInstance := cache.NewInMemory()
	fileManager := files.NewInMemoryFileManager()
	certsSvc := certificates.NewService(fileManager)
	nodesRepo := inmemory.NewNodeRepository()
	clientCertsRepo := inmemory.NewClientCertificateRepository()
	keyManager := enrollment.NewSetupKeyManager(cacheInstance, "")

	svc := enrollment.NewService(keyManager, nodesRepo, clientCertsRepo, certsSvc)

	return svc, cacheInstance
}

func TestGetHandler_no_key_configured(t *testing.T) {
	t.Parallel()
	svc, _ := setupEnrollmentService(t)
	handler := NewGetHandler(svc, api.NewResponder())

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/setup-key", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp setupKeyResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Empty(t, resp.SetupKey)
}

func TestGetHandler_key_exists(t *testing.T) {
	t.Parallel()
	svc, cacheInstance := setupEnrollmentService(t)
	handler := NewGetHandler(svc, api.NewResponder())

	err := cacheInstance.Set(context.Background(), enrollment.SetupKeyCacheKey, "existing-key")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/setup-key", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp setupKeyResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "existing-key", resp.SetupKey)
}

func TestPostHandler_generate_key(t *testing.T) {
	t.Parallel()
	svc, _ := setupEnrollmentService(t)
	handler := NewPostHandler(svc, api.NewResponder())

	req := httptest.NewRequest(http.MethodPost, "/api/nodes/setup-key", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp setupKeyResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.SetupKey)
	assert.Len(t, resp.SetupKey, 32)
}

func TestPostHandler_set_custom_key(t *testing.T) {
	t.Parallel()
	svc, _ := setupEnrollmentService(t)
	handler := NewPostHandler(svc, api.NewResponder())

	body, err := json.Marshal(map[string]string{"setup_key": "my-custom-key"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/nodes/setup-key", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp setupKeyResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "my-custom-key", resp.SetupKey)
}

func TestDeleteHandler(t *testing.T) {
	t.Parallel()
	svc, cacheInstance := setupEnrollmentService(t)
	handler := NewDeleteHandler(svc, api.NewResponder())

	err := cacheInstance.Set(context.Background(), enrollment.SetupKeyCacheKey, "key-to-delete")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete, "/api/nodes/setup-key", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	_, err = cacheInstance.Get(context.Background(), enrollment.SetupKeyCacheKey)
	assert.Error(t, err)
}
