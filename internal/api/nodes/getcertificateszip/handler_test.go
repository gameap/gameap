package getcertificateszip

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/certificates"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMockFileManager() *files.MockFileManager {
	storage := make(map[string][]byte)
	var mu sync.RWMutex

	return &files.MockFileManager{
		ReadFunc: func(_ context.Context, path string) ([]byte, error) {
			mu.RLock()
			defer mu.RUnlock()
			data, ok := storage[path]
			if !ok {
				return nil, nil
			}

			return data, nil
		},
		WriteFunc: func(_ context.Context, path string, data []byte) error {
			mu.Lock()
			defer mu.Unlock()
			storage[path] = data

			return nil
		},
		ExistsFunc: func(_ context.Context, path string) bool {
			mu.RLock()
			defer mu.RUnlock()
			_, ok := storage[path]

			return ok
		},
	}
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		expectedStatus int
		expectFiles    []string
	}{
		{
			name:           "successful certificate zip generation",
			expectedStatus: http.StatusOK,
			expectFiles:    []string{"ca.crt", "server.key", "server.crt", "README.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileManager := setupMockFileManager()
			certificatesSvc := certificates.NewService(fileManager)
			responder := api.NewResponder()
			handler := NewHandler(certificatesSvc, responder)

			req := httptest.NewRequest(http.MethodGet, "/api/dedicated_servers/certificates.zip", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Equal(t, "application/zip", w.Header().Get("Content-Type"))
			assert.Equal(t, "attachment; filename=\"certificates.zip\"", w.Header().Get("Content-Disposition"))

			zipReader, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
			require.NoError(t, err)

			fileNames := make(map[string]bool)
			for _, file := range zipReader.File {
				fileNames[file.Name] = true
			}

			for _, expectedFile := range tt.expectFiles {
				assert.True(t, fileNames[expectedFile], "expected file %s not found in zip", expectedFile)
			}
		})
	}
}

func TestHandler_GenerateCertificatesZip(t *testing.T) {
	fileManager := setupMockFileManager()
	certificatesSvc := certificates.NewService(fileManager)
	responder := api.NewResponder()
	handler := NewHandler(certificatesSvc, responder)

	ctx := context.Background()

	zipData, err := handler.generateCertificatesZip(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, zipData)

	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	require.NoError(t, err)

	expectedFiles := map[string]bool{
		"ca.crt":     false,
		"server.key": false,
		"server.crt": false,
		"README.md":  false,
	}

	for _, file := range zipReader.File {
		expectedFiles[file.Name] = true
	}

	for filename, found := range expectedFiles {
		assert.True(t, found, "expected file %s not found in zip", filename)
	}
}

func TestHandler_ZipFileContents(t *testing.T) {
	fileManager := setupMockFileManager()
	certificatesSvc := certificates.NewService(fileManager)
	responder := api.NewResponder()
	handler := NewHandler(certificatesSvc, responder)

	ctx := context.Background()

	zipData, err := handler.generateCertificatesZip(ctx)
	require.NoError(t, err)

	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	require.NoError(t, err)

	filesContent := make(map[string]string)
	for _, file := range zipReader.File {
		rc, err := file.Open()
		require.NoError(t, err)

		content, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())

		filesContent[file.Name] = string(content)
	}

	require.Contains(t, filesContent, "ca.crt")
	caCert := filesContent["ca.crt"]
	assert.NotEmpty(t, caCert)
	block, _ := pem.Decode([]byte(caCert))
	require.NotNil(t, block)
	assert.Equal(t, "CERTIFICATE", block.Type)
	_, err = x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	require.Contains(t, filesContent, "server.key")
	serverKey := filesContent["server.key"]
	assert.NotEmpty(t, serverKey)
	block, _ = pem.Decode([]byte(serverKey))
	require.NotNil(t, block)
	assert.Equal(t, "RSA PRIVATE KEY", block.Type)
	_, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	require.NoError(t, err)

	require.Contains(t, filesContent, "server.crt")
	serverCert := filesContent["server.crt"]
	assert.NotEmpty(t, serverCert)
	block, _ = pem.Decode([]byte(serverCert))
	require.NotNil(t, block)
	assert.Equal(t, "CERTIFICATE", block.Type)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	assert.Equal(t, "GameAP", cert.Subject.CommonName)
	assert.Contains(t, cert.Subject.Organization, "GameAP")

	require.Contains(t, filesContent, "README.md")
	readme := filesContent["README.md"]
	assert.NotEmpty(t, readme)
	assert.Contains(t, readme, "/etc/gameap-daemon/certs/")
	assert.Contains(t, readme, "ca_certificate_file")
	assert.Contains(t, readme, "certificate_chain_file")
	assert.Contains(t, readme, "private_key_file")
}

func TestHandler_NewHandler(t *testing.T) {
	fileManager := &files.MockFileManager{}
	certificatesSvc := certificates.NewService(fileManager)
	responder := api.NewResponder()

	handler := NewHandler(certificatesSvc, responder)

	require.NotNil(t, handler)
	assert.Equal(t, certificatesSvc, handler.certificatesSvc)
	assert.Equal(t, responder, handler.responder)
}

func TestHandler_MultipleCalls(t *testing.T) {
	fileManager := setupMockFileManager()
	certificatesSvc := certificates.NewService(fileManager)
	responder := api.NewResponder()
	handler := NewHandler(certificatesSvc, responder)

	ctx := context.Background()

	zipData1, err := handler.generateCertificatesZip(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, zipData1)

	zipData2, err := handler.generateCertificatesZip(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, zipData2)

	assert.NotEqual(t, zipData1, zipData2, "each call should generate different certificates")

	zipReader1, err := zip.NewReader(bytes.NewReader(zipData1), int64(len(zipData1)))
	require.NoError(t, err)

	zipReader2, err := zip.NewReader(bytes.NewReader(zipData2), int64(len(zipData2)))
	require.NoError(t, err)

	files1 := make(map[string]string)
	for _, file := range zipReader1.File {
		rc, err := file.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		files1[file.Name] = string(content)
	}

	files2 := make(map[string]string)
	for _, file := range zipReader2.File {
		rc, err := file.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		files2[file.Name] = string(content)
	}

	assert.Equal(t, files1["ca.crt"], files2["ca.crt"], "CA certificate should be the same")
	assert.Equal(t, files1["README.md"], files2["README.md"], "README should be the same")

	assert.NotEqual(t, files1["server.key"], files2["server.key"], "server key should be different")
	assert.NotEqual(t, files1["server.crt"], files2["server.crt"], "server cert should be different")
}
