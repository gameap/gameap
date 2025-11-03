package getclientcertificates

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const certificatePEM = `-----BEGIN CERTIFICATE-----
MIIDAjCCAeoCEF+513E1n9OiNa3D8VL+SCkwDQYJKoZIhvcNAQELBQAwMjELMAkG
A1UEBhMCUlUxDzANBgNVBAoMBkdhbWVBUDESMBAGA1UEAwwJR2FtZUFQIENBMB4X
DTI1MDkxODE4MjA1M1oXDTM1MDkxODE4MjA1M1owKDEPMA0GA1UECgwGR2FtZUFQ
MRUwEwYDVQQDDAxlMTE4M2JmODVjMGMwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAw
ggEKAoIBAQCsKDSASjS2YuRxfUB1Mg2QDx6iKKJ9gVCzFNtJ5zr8pTy4rA/LDW0u
B+IXkAubRjh70mk2A0rorOJRijzKGfm0dxjFXYyOUom1VrLqmnmOBEVDY4TtSi/9
MO3yI/Mt67+qPkLxcYjdn2WzQTnG25kbzjRrYNOAkxaZHOQ9/mjT+AFigoNqVpyy
6yWHcH5nsXYxnf/6CrS0leroHZFPqf6qQcjmObr3MVeJwZm8m2Ah4ZTeJsB10zFo
e6XCG5M5nqZF9rvdZ5g+9rvk8kmXD4FIBJ3aj9jYPnDglN/gILc0794/EMcIV+ho
OZ/Y4cKcHUq3T3NH4LTfFHs1dp3Gx+v7AgMBAAGjIzAhMB8GA1UdIwQYMBaAFLZ1
m82qyDalcjtcfIOBgtIZtn1zMA0GCSqGSIb3DQEBCwUAA4IBAQADrL/EEJl3w9sF
3nG17c6JwQolLfLu5RdYQfIVJYyixmsFszjDAg1kM4eCb4+863uNEWQIiENpOQKS
B7pNGot+qhSNWMIB2SG4I7t84cG3g3WWNPe+lrliveaP/f2RSga0HmnspaTv48vI
5v0KwMzxYJy6qkyA6BMA0Bh9JLp5sKWS2c2cwuVz4b4hg1I9vQXueZtPQ8+7dr06
rDh0OwALWVv74BMaUZgSE8FXazc2HWawSTfnPfidOAQBbOVfcWI7w73/fMiu6VQQ
ZEYUQZQX/+KjMkzodxA9x/cmw0tfRgn1HEhSJwkAyb6JC7w3/nyPxt13PGhb6QfC
aQnb1+Re
-----END CERTIFICATE-----`

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name             string
		setupRepo        func(*inmemory.ClientCertificateRepository)
		setupFileManager func(*files.MockFileManager)
		expectedStatus   int
		wantError        string
		expectCerts      bool
		expectedCount    int
	}{
		{
			name: "successful certificates retrieval",
			setupRepo: func(repo *inmemory.ClientCertificateRepository) {
				cert1 := &domain.ClientCertificate{
					ID:          1,
					Fingerprint: "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
					Expires:     time.Now().Add(365 * 24 * time.Hour),
					Certificate: "cert1",
					PrivateKey:  "key1",
				}
				cert2 := &domain.ClientCertificate{
					ID:          2,
					Fingerprint: "11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00",
					Expires:     time.Now().Add(180 * 24 * time.Hour),
					Certificate: "cert2",
					PrivateKey:  "key2",
				}

				require.NoError(t, repo.Save(context.Background(), cert1))
				require.NoError(t, repo.Save(context.Background(), cert2))
			},
			setupFileManager: func(repo *files.MockFileManager) {
				repo.ReadFunc = func(_ context.Context, _ string) ([]byte, error) {
					return []byte(certificatePEM), nil
				}
			},
			expectedStatus: http.StatusOK,
			expectCerts:    true,
			expectedCount:  2,
		},
		{
			name:           "no certificates available",
			setupRepo:      func(_ *inmemory.ClientCertificateRepository) {},
			expectedStatus: http.StatusOK,
			expectCerts:    true,
			expectedCount:  0,
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

			ctx := context.Background()
			req := httptest.NewRequest(http.MethodGet, "/api/client_certificates", nil)
			req = req.WithContext(ctx)
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

			if tt.expectCerts {
				var certs []clientCertificateResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &certs))
				assert.Len(t, certs, tt.expectedCount)

				if tt.expectedCount > 0 {
					cert := certs[0]
					assert.NotZero(t, cert.ID)
					assert.NotEmpty(t, cert.Fingerprint)
					assert.False(t, cert.Expires.IsZero())

					// Verify certificate info is populated
					assert.NotEmpty(t, cert.Info.CommonName)
					assert.NotEmpty(t, cert.Info.IssuerCommonName)
					assert.NotEmpty(t, cert.Info.SignatureType)
					assert.False(t, cert.Info.Expires.IsZero())
				}
			}
		})
	}
}

func TestHandler_CertificatesResponseFields(t *testing.T) {
	repo := inmemory.NewClientCertificateRepository()
	fileManager := &files.MockFileManager{
		ReadFunc: func(_ context.Context, _ string) ([]byte, error) {
			return []byte(certificatePEM), nil
		},
	}
	responder := api.NewResponder()
	handler := NewHandler(repo, fileManager, responder)

	expires := time.Now().Add(365 * 24 * time.Hour)
	cert := &domain.ClientCertificate{
		ID:          1,
		Fingerprint: "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
		Expires:     expires,
		Certificate: "cert1",
		PrivateKey:  "key1",
	}
	require.NoError(t, repo.Save(context.Background(), cert))

	req := httptest.NewRequest(http.MethodGet, "/api/client_certificates", nil)
	req = req.WithContext(context.Background())
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var certs []clientCertificateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &certs))

	require.Len(t, certs, 1)
	certResp := certs[0]

	assert.Equal(t, uint(1), certResp.ID)
	assert.Equal(t, "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99", certResp.Fingerprint)
	assert.WithinDuration(t, expires, certResp.Expires, time.Second)

	// Verify certificate info fields
	assert.Equal(t, "e1183bf85c0c", certResp.Info.CommonName)
	assert.Equal(t, "GameAP", certResp.Info.Organization)
	assert.Empty(t, certResp.Info.Country)
	assert.Empty(t, certResp.Info.State)
	assert.Empty(t, certResp.Info.Locality)
	assert.Empty(t, certResp.Info.OrganizationalUnit)
	assert.Empty(t, certResp.Info.Email)

	assert.Equal(t, "GameAP CA", certResp.Info.IssuerCommonName)
	assert.Equal(t, "GameAP", certResp.Info.IssuerOrganization)
	assert.Equal(t, "RU", certResp.Info.IssuerCountry)
	assert.Empty(t, certResp.Info.IssuerState)
	assert.Empty(t, certResp.Info.IssuerLocality)
	assert.Empty(t, certResp.Info.IssuerOrganizationalUnit)

	assert.Equal(t, "SHA256-RSA", certResp.Info.SignatureType)
	assert.False(t, certResp.Info.Expires.IsZero())
}

func TestHandler_CertificatesSortedByID(t *testing.T) {
	repo := inmemory.NewClientCertificateRepository()
	fileManager := &files.MockFileManager{
		ReadFunc: func(_ context.Context, _ string) ([]byte, error) {
			return []byte(certificatePEM), nil
		},
	}
	responder := api.NewResponder()
	handler := NewHandler(repo, fileManager, responder)

	cert3 := &domain.ClientCertificate{
		ID:          3,
		Fingerprint: "33:33:33:33:33:33:33:33:33:33:33:33:33:33:33:33",
		Expires:     time.Now().Add(365 * 24 * time.Hour),
		Certificate: "cert3",
		PrivateKey:  "key3",
	}
	cert1 := &domain.ClientCertificate{
		ID:          1,
		Fingerprint: "11:11:11:11:11:11:11:11:11:11:11:11:11:11:11:11",
		Expires:     time.Now().Add(365 * 24 * time.Hour),
		Certificate: "cert1",
		PrivateKey:  "key1",
	}
	cert2 := &domain.ClientCertificate{
		ID:          2,
		Fingerprint: "22:22:22:22:22:22:22:22:22:22:22:22:22:22:22:22",
		Expires:     time.Now().Add(365 * 24 * time.Hour),
		Certificate: "cert2",
		PrivateKey:  "key2",
	}

	require.NoError(t, repo.Save(context.Background(), cert3))
	require.NoError(t, repo.Save(context.Background(), cert1))
	require.NoError(t, repo.Save(context.Background(), cert2))

	req := httptest.NewRequest(http.MethodGet, "/api/client_certificates", nil)
	req = req.WithContext(context.Background())
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var certs []clientCertificateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &certs))

	require.Len(t, certs, 3)
	assert.Equal(t, uint(1), certs[0].ID)
	assert.Equal(t, uint(2), certs[1].ID)
	assert.Equal(t, uint(3), certs[2].ID)
}
