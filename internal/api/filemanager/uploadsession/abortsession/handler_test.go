package abortsession_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/filemanager/uploadsession/abortsession"
	"github.com/gameap/gameap/internal/api/filemanager/uploadsession/uploadsessiontest"
	"github.com/gameap/gameap/internal/upload"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
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
			name:        "success_returns_204",
			vars:        map[string]string{"server": "1", "uploadID": "u1"},
			auth:        true,
			grantAccess: true,
			serviceFactory: func(t *testing.T) *uploadsessiontest.FakeService {
				t.Helper()

				return &uploadsessiontest.FakeService{
					AbortFunc: func(_ context.Context, id string, uid uint) error {
						assert.Equal(t, "u1", id)
						assert.Equal(t, uploadsessiontest.UserID, uid)

						return nil
					},
				}
			},
			expectedStatus: http.StatusNoContent,
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
			name:        "session_forbidden_returns_403",
			vars:        map[string]string{"server": "1", "uploadID": "u1"},
			auth:        true,
			grantAccess: true,
			serviceFactory: func(*testing.T) *uploadsessiontest.FakeService {
				return &uploadsessiontest.FakeService{
					AbortFunc: func(context.Context, string, uint) error {
						return upload.ErrSessionForbidden
					},
				}
			},
			expectedStatus: http.StatusForbidden,
			wantError:      "belongs to another user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := uploadsessiontest.NewResolver(t, tt.grantAccess)
			handler := abortsession.NewHandler(resolver, tt.serviceFactory(t), api.NewResponder())

			req := uploadsessiontest.NewRequest(t, http.MethodDelete, nil, tt.vars, tt.auth)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.wantError != "" {
				uploadsessiontest.AssertErrorContains(t, w.Body.Bytes(), tt.wantError)
			}
		})
	}
}
