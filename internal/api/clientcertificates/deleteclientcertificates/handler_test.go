package deleteclientcertificates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name                 string
		certID               string
		setupRepo            func(*inmemory.ClientCertificateRepository)
		setupFileManager     func(*files.MockFileManager)
		expectedStatus       int
		expectFileDelete     bool
		expectedCertPath     string
		expectedKeyPath      string
		expectCertInRepoLeft bool
	}{
		{
			name:   "successful client certificate deletion",
			certID: "1",
			setupRepo: func(repo *inmemory.ClientCertificateRepository) {
				cert := &domain.ClientCertificate{
					ID:          1,
					Fingerprint: "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
					Expires:     time.Now().Add(365 * 24 * time.Hour),
					Certificate: "certs/client/test123.crt",
					PrivateKey:  "certs/client/test123.key",
				}
				require.NoError(t, repo.Save(context.Background(), cert))
			},
			setupFileManager: func(fm *files.MockFileManager) {
				deletedFiles := make([]string, 0)
				fm.DeleteFunc = func(_ context.Context, path string) error {
					deletedFiles = append(deletedFiles, path)

					return nil
				}
			},
			expectedStatus:       http.StatusNoContent,
			expectFileDelete:     true,
			expectedCertPath:     "certs/client/test123.crt",
			expectedKeyPath:      "certs/client/test123.key",
			expectCertInRepoLeft: false,
		},
		{
			name:   "delete non-existent client certificate",
			certID: "999",
			setupRepo: func(_ *inmemory.ClientCertificateRepository) {
			},
			setupFileManager: func(fm *files.MockFileManager) {
				fm.DeleteFunc = func(_ context.Context, _ string) error {
					t.Errorf("Delete should not be called for non-existent certificate")

					return nil
				}
			},
			expectedStatus:       http.StatusNoContent,
			expectFileDelete:     false,
			expectCertInRepoLeft: false,
		},
		{
			name:           "missing client certificate id",
			certID:         "",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid client certificate id - non-numeric",
			certID:         "invalid",
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "invalid client certificate id - negative",
			certID:         "-1",
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "invalid client certificate id - zero",
			certID:         "0",
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:   "file manager delete error should success",
			certID: "1",
			setupRepo: func(repo *inmemory.ClientCertificateRepository) {
				cert := &domain.ClientCertificate{
					ID:          1,
					Fingerprint: "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
					Expires:     time.Now().Add(365 * 24 * time.Hour),
					Certificate: "certs/client/test456.crt",
					PrivateKey:  "certs/client/test456.key",
				}
				require.NoError(t, repo.Save(context.Background(), cert))
			},
			setupFileManager: func(fm *files.MockFileManager) {
				fm.DeleteFunc = func(_ context.Context, _ string) error {
					return errors.New("file deletion failed")
				}
			},
			expectedStatus:       http.StatusNoContent,
			expectFileDelete:     true,
			expectCertInRepoLeft: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := inmemory.NewClientCertificateRepository()
			fileManager := &files.MockFileManager{}
			responder := api.NewResponder()
			handler := NewHandler(repo, fileManager, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}

			if tt.setupFileManager != nil {
				tt.setupFileManager(fileManager)
			}

			router := mux.NewRouter()
			router.Handle("/api/client_certificates/{id}", handler).Methods(http.MethodDelete)

			url := "/api/client_certificates/" + tt.certID
			if tt.certID == "" {
				url = "/api/client_certificates/"
			}

			req := httptest.NewRequest(http.MethodDelete, url, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.certID != "" && tt.certID != "invalid" && tt.certID != "-1" && tt.certID != "0" && tt.certID != "999" {
				allCerts, err := repo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				if tt.expectCertInRepoLeft {
					assert.Len(t, allCerts, 1)
				} else {
					assert.Len(t, allCerts, 0)
				}
			}
		})
	}
}

func TestHandler_CertificateDeletion(t *testing.T) {
	repo := inmemory.NewClientCertificateRepository()
	deletedFiles := make([]string, 0)
	fileManager := &files.MockFileManager{
		DeleteFunc: func(_ context.Context, path string) error {
			deletedFiles = append(deletedFiles, path)

			return nil
		},
	}
	responder := api.NewResponder()
	handler := NewHandler(repo, fileManager, responder)

	certs := []*domain.ClientCertificate{
		{
			ID:          1,
			Fingerprint: "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
			Expires:     time.Now().Add(365 * 24 * time.Hour),
			Certificate: "certs/client/cert1.crt",
			PrivateKey:  "certs/client/cert1.key",
		},
		{
			ID:          2,
			Fingerprint: "FF:EE:DD:CC:BB:AA:99:88:77:66:55:44:33:22:11:00:FF:EE:DD:CC:BB:AA:99:88:77:66:55:44:33:22:11:00",
			Expires:     time.Now().Add(180 * 24 * time.Hour),
			Certificate: "certs/client/cert2.crt",
			PrivateKey:  "certs/client/cert2.key",
		},
	}

	for _, cert := range certs {
		err := repo.Save(context.Background(), cert)
		require.NoError(t, err)
	}

	allCerts, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, allCerts, 2)

	router := mux.NewRouter()
	router.Handle("/api/client_certificates/{id}", handler).Methods(http.MethodDelete)

	req := httptest.NewRequest(http.MethodDelete, "/api/client_certificates/1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)

	allCerts, err = repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, allCerts, 1)

	assert.Equal(t, uint(2), allCerts[0].ID)
	assert.Equal(t, "FF:EE:DD:CC:BB:AA:99:88:77:66:55:44:33:22:11:00:FF:EE:DD:CC:BB:AA:99:88:77:66:55:44:33:22:11:00", allCerts[0].Fingerprint)

	require.Len(t, deletedFiles, 2)
	assert.Contains(t, deletedFiles, "certs/client/cert1.crt")
	assert.Contains(t, deletedFiles, "certs/client/cert1.key")
}

func TestHandler_IdempotentDeletion(t *testing.T) {
	repo := inmemory.NewClientCertificateRepository()
	deletedFiles := make([]string, 0)
	fileManager := &files.MockFileManager{
		DeleteFunc: func(_ context.Context, path string) error {
			deletedFiles = append(deletedFiles, path)

			return nil
		},
	}
	responder := api.NewResponder()
	handler := NewHandler(repo, fileManager, responder)

	cert := &domain.ClientCertificate{
		ID:          1,
		Fingerprint: "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
		Expires:     time.Now().Add(365 * 24 * time.Hour),
		Certificate: "certs/client/test.crt",
		PrivateKey:  "certs/client/test.key",
	}
	err := repo.Save(context.Background(), cert)
	require.NoError(t, err)

	router := mux.NewRouter()
	router.Handle("/api/client_certificates/{id}", handler).Methods(http.MethodDelete)

	req1 := httptest.NewRequest(http.MethodDelete, "/api/client_certificates/1", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusNoContent, w1.Code)

	allCerts, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, allCerts, 0)

	require.Len(t, deletedFiles, 2)

	req2 := httptest.NewRequest(http.MethodDelete, "/api/client_certificates/1", nil)
	w2 := httptest.NewRecorder()

	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusNoContent, w2.Code)

	assert.Len(t, deletedFiles, 2)
}

func TestHandler_FileManagerPartialFailure(t *testing.T) {
	repo := inmemory.NewClientCertificateRepository()
	deletedFiles := make([]string, 0)
	fileManager := &files.MockFileManager{
		DeleteFunc: func(_ context.Context, path string) error {
			if path == "certs/client/test.crt" {
				deletedFiles = append(deletedFiles, path)

				return errors.New("certificate file deletion failed")
			}
			deletedFiles = append(deletedFiles, path)

			return nil
		},
	}
	responder := api.NewResponder()
	handler := NewHandler(repo, fileManager, responder)

	cert := &domain.ClientCertificate{
		ID:          1,
		Fingerprint: "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
		Expires:     time.Now().Add(365 * 24 * time.Hour),
		Certificate: "certs/client/test.crt",
		PrivateKey:  "certs/client/test.key",
	}
	err := repo.Save(context.Background(), cert)
	require.NoError(t, err)

	router := mux.NewRouter()
	router.Handle("/api/client_certificates/{id}", handler).Methods(http.MethodDelete)

	req := httptest.NewRequest(http.MethodDelete, "/api/client_certificates/1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	allCerts, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, allCerts, 0)

	require.Len(t, deletedFiles, 2)
}

func TestHandler_NewHandler(t *testing.T) {
	repo := inmemory.NewClientCertificateRepository()
	fileManager := &files.MockFileManager{}
	responder := api.NewResponder()

	handler := NewHandler(repo, fileManager, responder)

	require.NotNil(t, handler)
	assert.Equal(t, repo, handler.repo)
	assert.Equal(t, fileManager, handler.fileManager)
	assert.Equal(t, responder, handler.responder)
}
