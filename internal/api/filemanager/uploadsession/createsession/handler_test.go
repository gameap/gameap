package createsession_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/filemanager/uploadsession/createsession"
	"github.com/gameap/gameap/internal/api/filemanager/uploadsession/uploadsessiontest"
	"github.com/gameap/gameap/internal/upload"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()

	validBody := `{"path":"configs","filename":"big.bin","total_size":100,"expected_checksum":"` +
		strings.Repeat("a", 64) + `"}`

	tests := []struct {
		name           string
		body           string
		auth           bool
		grantAccess    bool
		serverID       string
		serviceFactory func(t *testing.T) *uploadsessiontest.FakeService
		expectedStatus int
		wantError      string
		validate       func(t *testing.T, body []byte)
	}{
		{
			name:        "success_creates_session_returns_201",
			body:        validBody,
			auth:        true,
			grantAccess: true,
			serverID:    "1",
			serviceFactory: func(t *testing.T) *uploadsessiontest.FakeService {
				t.Helper()

				return &uploadsessiontest.FakeService{
					CreateFunc: func(_ context.Context, p upload.CreateParams) (*upload.Session, error) {
						assert.Equal(t, uploadsessiontest.UserID, p.UserID)
						assert.Equal(t, uploadsessiontest.ServerID, p.ServerID)
						assert.Equal(t, uploadsessiontest.NodeID, p.NodeID)
						assert.Equal(t, "/srv/gameap/servers/test1/configs/big.bin", p.FullPath)
						assert.Equal(t, uint64(100), p.TotalSize)

						return &upload.Session{
							UploadID:    "upload-xyz",
							ChunkSize:   8 << 20,
							TotalChunks: 1,
							TotalSize:   100,
							ExpiresAt:   time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC),
						}, nil
					},
				}
			},
			expectedStatus: http.StatusCreated,
			validate: func(t *testing.T, body []byte) {
				t.Helper()
				var resp createsession.Response
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, "upload-xyz", resp.UploadID)
				assert.Equal(t, uint64(8<<20), resp.ChunkSize)
				assert.Equal(t, uint(1), resp.TotalChunks)
				assert.Equal(t, uint64(100), resp.TotalSize)
				assert.Equal(t, time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC), resp.ExpiresAt)
			},
		},
		{
			name:           "unauthorized_when_no_session",
			body:           validBody,
			auth:           false,
			serverID:       "1",
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:           "forbidden_without_files_ability",
			body:           validBody,
			auth:           true,
			grantAccess:    false,
			serverID:       "1",
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusForbidden,
			wantError:      "user does not have required permissions",
		},
		{
			name:           "bad_request_on_invalid_path",
			body:           `{"path":"../etc","filename":"x","total_size":1,"expected_checksum":"` + strings.Repeat("a", 64) + `"}`,
			auth:           true,
			grantAccess:    true,
			serverID:       "1",
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusBadRequest,
			wantError:      "path contains invalid directory traversal",
		},
		{
			name:           "bad_request_on_empty_filename",
			body:           `{"path":"configs","filename":"","total_size":1,"expected_checksum":"` + strings.Repeat("a", 64) + `"}`,
			auth:           true,
			grantAccess:    true,
			serverID:       "1",
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusBadRequest,
			wantError:      "filename is empty",
		},
		{
			name:           "bad_request_on_missing_total_size",
			body:           `{"path":"configs","filename":"a","total_size":0,"expected_checksum":"` + strings.Repeat("a", 64) + `"}`,
			auth:           true,
			grantAccess:    true,
			serverID:       "1",
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusBadRequest,
			wantError:      "total_size must be positive",
		},
		{
			name:           "bad_request_on_missing_expected_checksum",
			body:           `{"path":"configs","filename":"a","total_size":1,"expected_checksum":""}`,
			auth:           true,
			grantAccess:    true,
			serverID:       "1",
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusBadRequest,
			wantError:      "expected_checksum is required",
		},
		{
			name:           "bad_request_on_invalid_json_body",
			body:           `{`,
			auth:           true,
			grantAccess:    true,
			serverID:       "1",
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid request body",
		},
		{
			name:           "bad_request_on_invalid_server_id",
			body:           validBody,
			auth:           true,
			grantAccess:    true,
			serverID:       "abc",
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
		},
		{
			name:           "not_found_when_server_unknown",
			body:           validBody,
			auth:           true,
			grantAccess:    true,
			serverID:       "999",
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
		},
		{
			name:        "service_invalid_checksum_returns_400",
			body:        validBody,
			auth:        true,
			grantAccess: true,
			serverID:    "1",
			serviceFactory: func(*testing.T) *uploadsessiontest.FakeService {
				return &uploadsessiontest.FakeService{
					CreateFunc: func(context.Context, upload.CreateParams) (*upload.Session, error) {
						return nil, upload.ErrInvalidChecksum
					},
				}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "expected_checksum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := uploadsessiontest.NewResolver(t, tt.grantAccess)
			handler := createsession.NewHandler(resolver, tt.serviceFactory(t), api.NewResponder())

			req := uploadsessiontest.NewRequest(t, http.MethodPost, []byte(tt.body),
				map[string]string{"server": tt.serverID}, tt.auth)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.wantError != "" {
				uploadsessiontest.AssertErrorContains(t, w.Body.Bytes(), tt.wantError)
			}
			if tt.validate != nil {
				tt.validate(t, w.Body.Bytes())
			}
		})
	}
}
