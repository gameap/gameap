package completesession_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/filemanager/uploadsession/completesession"
	"github.com/gameap/gameap/internal/api/filemanager/uploadsession/uploadsessiontest"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/upload"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
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
	}{
		{
			name:        "success_dispatches_returns_200",
			vars:        map[string]string{"server": "1", "uploadID": "u1"},
			auth:        true,
			grantAccess: true,
			serviceFactory: func(t *testing.T) *uploadsessiontest.FakeService {
				t.Helper()

				return &uploadsessiontest.FakeService{
					CompleteFunc: func(_ context.Context, id string, uid uint, node *domain.Node) error {
						assert.Equal(t, "u1", id)
						assert.Equal(t, uploadsessiontest.UserID, uid)
						require.NotNil(t, node)
						assert.Equal(t, uploadsessiontest.NodeID, node.ID)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
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
			name:        "checksum_mismatch_returns_422",
			vars:        map[string]string{"server": "1", "uploadID": "u1"},
			auth:        true,
			grantAccess: true,
			serviceFactory: func(*testing.T) *uploadsessiontest.FakeService {
				return &uploadsessiontest.FakeService{
					CompleteFunc: func(context.Context, string, uint, *domain.Node) error {
						return upload.ErrChecksumMismatch
					},
				}
			},
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "checksum",
		},
		{
			name:        "incomplete_returns_409",
			vars:        map[string]string{"server": "1", "uploadID": "u1"},
			auth:        true,
			grantAccess: true,
			serviceFactory: func(*testing.T) *uploadsessiontest.FakeService {
				return &uploadsessiontest.FakeService{
					CompleteFunc: func(context.Context, string, uint, *domain.Node) error {
						return upload.ErrIncompleteUpload
					},
				}
			},
			expectedStatus: http.StatusConflict,
			wantError:      "not all chunks",
		},
		{
			name:        "node_mismatch_returns_403",
			vars:        map[string]string{"server": "1", "uploadID": "u1"},
			auth:        true,
			grantAccess: true,
			serviceFactory: func(*testing.T) *uploadsessiontest.FakeService {
				return &uploadsessiontest.FakeService{
					CompleteFunc: func(context.Context, string, uint, *domain.Node) error {
						return upload.ErrNodeMismatch
					},
				}
			},
			expectedStatus: http.StatusForbidden,
			wantError:      "node does not match",
		},
		{
			name:        "daemon_error_returns_500",
			vars:        map[string]string{"server": "1", "uploadID": "u1"},
			auth:        true,
			grantAccess: true,
			serviceFactory: func(*testing.T) *uploadsessiontest.FakeService {
				return &uploadsessiontest.FakeService{
					CompleteFunc: func(context.Context, string, uint, *domain.Node) error {
						return errors.New("daemon down")
					},
				}
			},
			expectedStatus: http.StatusInternalServerError,
			// 5xx bodies are scrubbed by api.Responder to "Internal Server Error"
			// (see pkg/api/responder.go) — assert that contract holds.
			wantError: "Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := uploadsessiontest.NewResolver(t, tt.grantAccess)
			handler := completesession.NewHandler(resolver, tt.serviceFactory(t), api.NewResponder())

			req := uploadsessiontest.NewRequest(t, http.MethodPost, nil, tt.vars, tt.auth)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.wantError != "" {
				uploadsessiontest.AssertErrorContains(t, w.Body.Bytes(), tt.wantError)
			}
			if tt.expectedStatus == http.StatusOK {
				var resp completesession.Response
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				assert.Equal(t, "u1", resp.UploadID)
				assert.True(t, resp.Completed)
			}
		})
	}
}
