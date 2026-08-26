package plugininstall_test

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/services/plugininstall"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateWASM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		data      []byte
		wantError string
	}{
		{
			name:      "valid_wasm",
			data:      []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00},
			wantError: "",
		},
		{
			name:      "too_small",
			data:      []byte{0x00, 0x61},
			wantError: "file too small to be valid WASM",
		},
		{
			name:      "empty",
			data:      []byte{},
			wantError: "file too small to be valid WASM",
		},
		{
			name:      "invalid_magic",
			data:      []byte{0x01, 0x02, 0x03, 0x04},
			wantError: "invalid WASM magic number",
		},
		{
			name:      "exactly_4_bytes_invalid",
			data:      []byte{0x00, 0x61, 0x73, 0x00},
			wantError: "invalid WASM magic number",
		},
		{
			name:      "exactly_4_bytes_valid",
			data:      []byte{0x00, 0x61, 0x73, 0x6d},
			wantError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := plugininstall.ValidateWASM(tt.data)

			if tt.wantError == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			}
		})
	}
}

func createMultipartRequest(t *testing.T, fieldName, filename string, content []byte) *http.Request {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, filename)
	require.NoError(t, err)

	_, err = io.Copy(part, bytes.NewReader(content))
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req
}

func TestReadWASMFromMultipart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupReq  func(t *testing.T) (*http.Request, *httptest.ResponseRecorder)
		wantBytes []byte
		wantError string
	}{
		{
			name: "valid_upload",
			setupReq: func(t *testing.T) (*http.Request, *httptest.ResponseRecorder) {
				t.Helper()
				req := createMultipartRequest(t, "file", "plugin.wasm", []byte{0x00, 0x61, 0x73, 0x6d})

				return req, httptest.NewRecorder()
			},
			wantBytes: []byte{0x00, 0x61, 0x73, 0x6d},
		},
		{
			name: "no_file_field",
			setupReq: func(t *testing.T) (*http.Request, *httptest.ResponseRecorder) {
				t.Helper()
				req := createMultipartRequest(t, "other_field", "plugin.wasm", []byte{0x00, 0x61, 0x73, 0x6d})

				return req, httptest.NewRecorder()
			},
			wantError: "no file uploaded",
		},
		{
			name: "no_multipart_form",
			setupReq: func(t *testing.T) (*http.Request, *httptest.ResponseRecorder) {
				t.Helper()
				req := httptest.NewRequest(http.MethodPost, "/upload", nil)
				req.Header.Set("Content-Type", "multipart/form-data")

				return req, httptest.NewRecorder()
			},
			wantError: "failed to parse multipart form",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, rw := tt.setupReq(t)

			gotBytes, err := plugininstall.ReadWASMFromMultipart(rw, req)

			if tt.wantError == "" {
				require.NoError(t, err)
				assert.Equal(t, tt.wantBytes, gotBytes)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			}
		})
	}
}
