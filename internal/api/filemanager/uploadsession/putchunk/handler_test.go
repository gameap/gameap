package putchunk_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/filemanager/uploadsession/putchunk"
	"github.com/gameap/gameap/internal/api/filemanager/uploadsession/uploadsessiontest"
	"github.com/gameap/gameap/internal/upload"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testChunkSize = 8 << 20

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		body           []byte
		vars           map[string]string
		auth           bool
		grantAccess    bool
		serviceFactory func(t *testing.T) *uploadsessiontest.FakeService
		expectedStatus int
		wantError      string
	}{
		{
			name:        "success_writes_chunk_returns_204",
			body:        bytes.Repeat([]byte("X"), 10),
			vars:        map[string]string{"server": "1", "uploadID": "u1", "index": "3"},
			auth:        true,
			grantAccess: true,
			serviceFactory: func(t *testing.T) *uploadsessiontest.FakeService {
				t.Helper()

				return &uploadsessiontest.FakeService{
					WriteFunc: func(_ context.Context, id string, uid, idx uint, body io.Reader) error {
						assert.Equal(t, "u1", id)
						assert.Equal(t, uploadsessiontest.UserID, uid)
						assert.Equal(t, uint(3), idx)
						read, err := io.ReadAll(body)
						require.NoError(t, err)
						require.Len(t, read, 10)

						return nil
					},
				}
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "unauthorized_when_no_session",
			body:           []byte("xxxx"),
			vars:           map[string]string{"server": "1", "uploadID": "u1", "index": "0"},
			auth:           false,
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:           "forbidden_without_files_ability",
			body:           []byte("xxxx"),
			vars:           map[string]string{"server": "1", "uploadID": "u1", "index": "0"},
			auth:           true,
			grantAccess:    false,
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusForbidden,
			wantError:      "user does not have required permissions",
		},
		{
			name:           "bad_request_when_index_invalid",
			body:           []byte("xxxx"),
			vars:           map[string]string{"server": "1", "uploadID": "u1", "index": "abc"},
			auth:           true,
			grantAccess:    true,
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid chunk index",
		},
		{
			name:           "bad_request_when_server_id_invalid",
			body:           []byte("xxxx"),
			vars:           map[string]string{"server": "abc", "uploadID": "u1", "index": "0"},
			auth:           true,
			grantAccess:    true,
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
		},
		{
			name:           "missing_upload_id",
			body:           []byte("xxxx"),
			vars:           map[string]string{"server": "1", "uploadID": "", "index": "0"},
			auth:           true,
			grantAccess:    true,
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusBadRequest,
			wantError:      "uploadID is required",
		},
		{
			name:        "session_forbidden_returns_403",
			body:        []byte("xxxx"),
			vars:        map[string]string{"server": "1", "uploadID": "u1", "index": "0"},
			auth:        true,
			grantAccess: true,
			serviceFactory: func(*testing.T) *uploadsessiontest.FakeService {
				return &uploadsessiontest.FakeService{
					WriteFunc: func(context.Context, string, uint, uint, io.Reader) error {
						return upload.ErrSessionForbidden
					},
				}
			},
			expectedStatus: http.StatusForbidden,
			wantError:      "belongs to another user",
		},
		{
			name:        "chunk_size_mismatch_returns_413",
			body:        []byte("xxxx"),
			vars:        map[string]string{"server": "1", "uploadID": "u1", "index": "0"},
			auth:        true,
			grantAccess: true,
			serviceFactory: func(*testing.T) *uploadsessiontest.FakeService {
				return &uploadsessiontest.FakeService{
					WriteFunc: func(context.Context, string, uint, uint, io.Reader) error {
						return upload.ErrChunkSizeMismatch
					},
				}
			},
			expectedStatus: http.StatusRequestEntityTooLarge,
			wantError:      "chunk size",
		},
		{
			name:        "session_not_found_returns_404",
			body:        []byte("xxxx"),
			vars:        map[string]string{"server": "1", "uploadID": "missing", "index": "0"},
			auth:        true,
			grantAccess: true,
			serviceFactory: func(*testing.T) *uploadsessiontest.FakeService {
				return &uploadsessiontest.FakeService{
					WriteFunc: func(context.Context, string, uint, uint, io.Reader) error {
						return upload.ErrSessionNotFound
					},
				}
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := uploadsessiontest.NewResolver(t, tt.grantAccess)
			handler := putchunk.NewHandler(resolver, tt.serviceFactory(t), api.NewResponder(), testChunkSize)

			req := uploadsessiontest.NewRequest(t, http.MethodPut, tt.body, tt.vars, tt.auth)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.wantError != "" {
				uploadsessiontest.AssertErrorContains(t, w.Body.Bytes(), tt.wantError)
			}
		})
	}
}
