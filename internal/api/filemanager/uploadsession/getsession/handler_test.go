package getsession_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/filemanager/uploadsession/getsession"
	"github.com/gameap/gameap/internal/api/filemanager/uploadsession/uploadsessiontest"
	"github.com/gameap/gameap/internal/upload"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		vars           map[string]string
		auth           bool
		grantAccess    bool
		serviceFactory func(t *testing.T) *uploadsessiontest.FakeService
		expectedStatus int
		wantError      string
		validate       func(t *testing.T, body []byte)
	}{
		{
			name:        "success_returns_status_with_received_chunks",
			vars:        map[string]string{"server": "1", "uploadID": "u1"},
			auth:        true,
			grantAccess: true,
			serviceFactory: func(t *testing.T) *uploadsessiontest.FakeService {
				t.Helper()

				return &uploadsessiontest.FakeService{
					StatusFunc: func(_ context.Context, id string, uid uint) (*upload.SessionStatus, error) {
						assert.Equal(t, "u1", id)
						assert.Equal(t, uploadsessiontest.UserID, uid)

						return &upload.SessionStatus{
							Session: &upload.Session{
								UploadID:    "u1",
								TotalSize:   100,
								ChunkSize:   30,
								TotalChunks: 4,
								ExpiresAt:   time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC),
							},
							ReceivedChunks: []uint{0, 2},
							MissingChunks:  []uint{1, 3},
							UploadedBytes:  60,
							Completed:      false,
						}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, body []byte) {
				t.Helper()
				var resp getsession.Response
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, "u1", resp.UploadID)
				assert.Equal(t, uint64(100), resp.TotalSize)
				assert.Equal(t, uint64(30), resp.ChunkSize)
				assert.Equal(t, uint(4), resp.TotalChunks)
				assert.Equal(t, []uint{0, 2}, resp.ReceivedChunks)
				assert.Equal(t, []uint{1, 3}, resp.MissingChunks)
				assert.Equal(t, uint64(60), resp.UploadedBytes)
				assert.False(t, resp.Completed)
				assert.Equal(t, time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC), resp.ExpiresAt)
			},
		},
		{
			name:        "encodes_empty_arrays_when_no_chunks",
			vars:        map[string]string{"server": "1", "uploadID": "u1"},
			auth:        true,
			grantAccess: true,
			serviceFactory: func(*testing.T) *uploadsessiontest.FakeService {
				return &uploadsessiontest.FakeService{
					StatusFunc: func(context.Context, string, uint) (*upload.SessionStatus, error) {
						return &upload.SessionStatus{
							Session:   &upload.Session{UploadID: "u1", TotalChunks: 1, ChunkSize: 100, TotalSize: 100},
							Completed: false,
						}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, body []byte) {
				t.Helper()
				assert.Contains(t, string(body), `"received_chunks":[]`,
					"nil ReceivedChunks must serialize as [] for stable client contract")
				assert.Contains(t, string(body), `"missing_chunks":[]`,
					"nil MissingChunks must serialize as [] for stable client contract")
			},
		},
		{
			name:           "unauthorized_when_no_session",
			vars:           map[string]string{"server": "1", "uploadID": "u1"},
			auth:           false,
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:           "forbidden_without_ability",
			vars:           map[string]string{"server": "1", "uploadID": "u1"},
			auth:           true,
			grantAccess:    false,
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusForbidden,
			wantError:      "user does not have required permissions",
		},
		{
			name:           "bad_request_when_server_id_invalid",
			vars:           map[string]string{"server": "abc", "uploadID": "u1"},
			auth:           true,
			grantAccess:    true,
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
		},
		{
			name:           "bad_request_when_upload_id_missing",
			vars:           map[string]string{"server": "1", "uploadID": ""},
			auth:           true,
			grantAccess:    true,
			serviceFactory: uploadsessiontest.EmptyService,
			expectedStatus: http.StatusBadRequest,
			wantError:      "uploadID is required",
		},
		{
			name:        "not_found_when_session_unknown",
			vars:        map[string]string{"server": "1", "uploadID": "missing"},
			auth:        true,
			grantAccess: true,
			serviceFactory: func(*testing.T) *uploadsessiontest.FakeService {
				return &uploadsessiontest.FakeService{
					StatusFunc: func(context.Context, string, uint) (*upload.SessionStatus, error) {
						return nil, upload.ErrSessionNotFound
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
			handler := getsession.NewHandler(resolver, tt.serviceFactory(t), api.NewResponder())

			req := uploadsessiontest.NewRequest(t, http.MethodGet, nil, tt.vars, tt.auth)
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
