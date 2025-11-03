package postclientcertificates

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	validCertPEM = `-----BEGIN CERTIFICATE-----
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

	validKeyPEM = `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQCsKDSASjS2YuRx
fUB1Mg2QDx6iKKJ9gVCzFNtJ5zr8pTy4rA/LDW0uB+IXkAubRjh70mk2A0rorOJR
ijzKGfm0dxjFXYyOUom1VrLqmnmOBEVDY4TtSi/9MO3yI/Mt67+qPkLxcYjdn2Wz
QTnG25kbzjRrYNOAkxaZHOQ9/mjT+AFigoNqVpyy6yWHcH5nsXYxnf/6CrS0lero
HZFPqf6qQcjmObr3MVeJwZm8m2Ah4ZTeJsB10zFoe6XCG5M5nqZF9rvdZ5g+9rvk
8kmXD4FIBJ3aj9jYPnDglN/gILc0794/EMcIV+hoOZ/Y4cKcHUq3T3NH4LTfFHs1
dp3Gx+v7AgMBAAECggEAD+Hv3J0s8wZCE/Hs+BClcEqIIx9O/DM6PKDP8Yv0tZK4
Ip/ehjBh+bO5BQPYz6aOjRq8wqNpMqqJ7BJFnOxPCL8Q3Q7Mg8HqZ9vJLx0jQWkr
wCcZNOcOwOGVQqDScr+6lWKrH+bSLh3CGt8RlI1tWJZGqvR8FnNkz9LsJJM8vcCE
9LvXXqLGiWQ3GxH7bXx4KdqBaH4BqlZHMnNwwE/qB4fQ8aEE4QhSK1GqaGYvNzLk
CqxVPQXdE7BL3BWqHQHDqQYdLqIkBF9rPvNLqKEQNqJBcEqR+YHRVxPQl3q8VU4b
LYw0WUa9bD6aRPr5XfG3+TqBXbvBqd8K9L3iUdGRAQKBgQDYBatB6xVH7Dj7Vzwb
iyqF9TBsq0pHFqE3yfJGF0oCKx9RcpE7E7u8v5BJLWOpO3rLa+VEyqcJX+6vhMdQ
8J7fJg0f7BsqF2G8bHmJ5KqLiKZ3bwqFN6RDOoQqGx+p6K7S6UHq3JGIiBPcPqED
QnVGa7KELqPpYLxY2TdYvbRy+wKBgQDMPYJeELCvMRVq7lVKa6jN7qLBH7bZvz3Q
TQXsJmcX/qOv3K0qLKDQJ0pYXH2qGN0VF9Q3nLQJX8qKZqNLQJ7HbXFQ8JYGQ3OG
PKx5lGqZd6JQJf8D9YqX3bQvKQX8O3Jq0bvVQF3pYLN6vB8VFqLQJ0pYXH2qGN0V
F9Q3nLQJXQKBgQCJp5vJHBYXqQJXH2qGN0VF9Q3nLQJX8qKZqNLQJ7HbXFQ8JYGQ
3OGPKx5lGqZd6JQJf8D9YqX3bQvKQX8O3Jq0bvVQF3pYLN6vB8VFqLQJ0pYXH2qG
N0VF9Q3nLQJX8qKZqNLQJ7HbXFQ8JYGQ3OGPKx5lGqZd6JQJf8D9YqX3bQvKQX8O
3Jq0bvVQF3pYLQKBgBJXH2qGN0VF9Q3nLQJX8qKZqNLQJ7HbXFQ8JYGQ3OGPKx5l
GqZd6JQJf8D9YqX3bQvKQX8O3Jq0bvVQF3pYLN6vB8VFqLQJ0pYXH2qGN0VF9Q3n
LQJX8qKZqNLQJ7HbXFQ8JYGQ3OGPKx5lGqZd6JQJf8D9YqX3bQvKQX8O3Jq0bvVQ
F3pYLN6vB8VFqLQJAoGBAMlGqZd6JQJf8D9YqX3bQvKQX8O3Jq0bvVQF3pYLN6vB
8VFqLQJ0pYXH2qGN0VF9Q3nLQJX8qKZqNLQJ7HbXFQ8JYGQ3OGPKx5lGqZd6JQJf
8D9YqX3bQvKQX8O3Jq0bvVQF3pYLN6vB8VFqLQJ0pYXH2qGN0VF9Q3nLQJX8qKZq
NLQJ7HbXFQ8JYGQ3OGPK
-----END PRIVATE KEY-----`

	invalidCertPEM = `-----BEGIN CERTIFICATE-----
INVALID CERTIFICATE DATA
-----END CERTIFICATE-----`

	invalidKeyPEM = `-----BEGIN PRIVATE KEY-----
INVALID KEY DATA
-----END PRIVATE KEY-----`
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name             string
		setupRequest     func() *http.Request
		setupFileManager func(*files.MockFileManager)
		expectedStatus   int
		expectError      bool
		expectCertID     bool
	}{
		{
			name: "successful certificate upload",
			setupRequest: func() *http.Request {
				return createMultipartRequest(t, validCertPEM, validKeyPEM, "test123")
			},
			setupFileManager: func(fm *files.MockFileManager) {
				fm.WriteFunc = func(_ context.Context, _ string, _ []byte) error {
					return nil
				}
			},
			expectedStatus: http.StatusCreated,
			expectCertID:   true,
		},
		{
			name: "certificate file missing",
			setupRequest: func() *http.Request {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)

				keyPart, _ := writer.CreateFormFile(fieldPrivateKey, "test.key")
				_, _ = io.WriteString(keyPart, validKeyPEM)

				_ = writer.WriteField(fieldPrivateKeyPass, "test123")
				_ = writer.Close()

				req := httptest.NewRequest(http.MethodPost, "/api/client_certificates", body)
				req.Header.Set("Content-Type", writer.FormDataContentType())

				return req
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name: "private key file missing",
			setupRequest: func() *http.Request {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)

				certPart, _ := writer.CreateFormFile(fieldCertificate, "test.crt")
				_, _ = io.WriteString(certPart, validCertPEM)

				_ = writer.WriteField(fieldPrivateKeyPass, "test123")
				_ = writer.Close()

				req := httptest.NewRequest(http.MethodPost, "/api/client_certificates", body)
				req.Header.Set("Content-Type", writer.FormDataContentType())

				return req
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    true,
		},
		{
			name: "invalid certificate format",
			setupRequest: func() *http.Request {
				return createMultipartRequest(t, invalidCertPEM, validKeyPEM, "test123")
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "invalid private key format",
			setupRequest: func() *http.Request {
				return createMultipartRequest(t, validCertPEM, invalidKeyPEM, "test123")
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "file manager write error",
			setupRequest: func() *http.Request {
				return createMultipartRequest(t, validCertPEM, validKeyPEM, "test123")
			},
			setupFileManager: func(fm *files.MockFileManager) {
				fm.WriteFunc = func(_ context.Context, _ string, _ []byte) error {
					return assert.AnError
				}
				fm.DeleteFunc = func(_ context.Context, _ string) error {
					return nil
				}
			},
			expectedStatus: http.StatusInternalServerError,
			expectError:    true,
		},
		{
			name: "upload without password",
			setupRequest: func() *http.Request {
				return createMultipartRequest(t, validCertPEM, validKeyPEM, "")
			},
			setupFileManager: func(fm *files.MockFileManager) {
				fm.WriteFunc = func(_ context.Context, _ string, _ []byte) error {
					return nil
				}
			},
			expectedStatus: http.StatusCreated,
			expectCertID:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := inmemory.NewClientCertificateRepository()
			fileManager := &files.MockFileManager{}
			responder := api.NewResponder()

			if tt.setupFileManager != nil {
				tt.setupFileManager(fileManager)
			}

			handler := NewHandler(repo, fileManager, responder)

			req := tt.setupRequest()
			req = req.WithContext(context.Background())
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectError {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
			}

			if tt.expectCertID {
				var response clientCertificateResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.NotZero(t, response.ID)
			}
		})
	}
}

func TestHandler_CertificateStoredCorrectly(t *testing.T) {
	repo := inmemory.NewClientCertificateRepository()
	fileManager := &files.MockFileManager{}
	responder := api.NewResponder()
	handler := NewHandler(repo, fileManager, responder)

	req := createMultipartRequest(t, validCertPEM, validKeyPEM, "test123")
	req = req.WithContext(context.Background())
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var response clientCertificateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	certs, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, certs, 1)

	cert := certs[0]
	assert.Equal(t, response.ID, cert.ID)
	assert.NotEmpty(t, cert.Fingerprint)
	assert.False(t, cert.Expires.IsZero())
	assert.Contains(t, cert.Certificate, "certs/client/")
	assert.Contains(t, cert.PrivateKey, "certs/client/")
	assert.True(t, cert.Expires.After(time.Now()))
}

func TestHandler_FingerprintCalculation(t *testing.T) {
	repo := inmemory.NewClientCertificateRepository()
	fileManager := &files.MockFileManager{}
	responder := api.NewResponder()
	handler := NewHandler(repo, fileManager, responder)

	req := createMultipartRequest(t, validCertPEM, validKeyPEM, "")
	req = req.WithContext(context.Background())
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	certs, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, certs, 1)

	cert := certs[0]
	assert.Regexp(t, `^([A-F0-9]{2}:){31}[A-F0-9]{2}$`, cert.Fingerprint)
}

func createMultipartRequest(t *testing.T, certContent, keyContent, password string) *http.Request {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	certPart, err := writer.CreateFormFile(fieldCertificate, "test.crt")
	require.NoError(t, err)
	_, err = io.WriteString(certPart, certContent)
	require.NoError(t, err)

	keyPart, err := writer.CreateFormFile(fieldPrivateKey, "test.key")
	require.NoError(t, err)
	_, err = io.WriteString(keyPart, keyContent)
	require.NoError(t, err)

	if password != "" {
		err = writer.WriteField(fieldPrivateKeyPass, password)
		require.NoError(t, err)
	}

	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/client_certificates", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req
}
