package streamfile

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statOnlyGateway is a FileGateway whose daemon answers every stat with a
// canned FileOperationResponse; other calls are never expected here.
type statOnlyGateway struct {
	statResp *proto.FileOperationResponse
	statPath string
}

func (g *statOnlyGateway) RequestFileOperation(
	_ context.Context, _ uint64, req *proto.FileOperationRequest,
) (*proto.FileOperationResponse, error) {
	g.statPath = req.GetStatParams().GetPath()

	return g.statResp, nil
}

func (g *statOnlyGateway) RequestFileList(
	context.Context, uint64, string, bool, string,
) (*proto.FileListResponse, error) {
	return nil, errors.New("unexpected RequestFileList")
}

func (g *statOnlyGateway) RequestFileRead(
	context.Context, uint64, string, int64, int64,
) (*proto.FileReadResponse, error) {
	return nil, errors.New("unexpected RequestFileRead")
}

func (g *statOnlyGateway) RequestFileWrite(
	context.Context, uint64, string, []byte, int32, bool, daemon.OwnerOptions,
) error {
	return errors.New("unexpected RequestFileWrite")
}

func (g *statOnlyGateway) RequestFileUploadTask(
	context.Context, uint64, string, string, string, int64, int32, daemon.OwnerOptions,
) error {
	return errors.New("unexpected RequestFileUploadTask")
}

func (g *statOnlyGateway) RequestFileDownloadTask(context.Context, uint64, string, string) error {
	return errors.New("unexpected RequestFileDownloadTask")
}

type alwaysConnected struct{}

func (alwaysConnected) IsConnected(uint64) bool         { return true }
func (alwaysConnected) IsConnectedAnywhere(uint64) bool { return true }
func (alwaysConnected) HasCapability(uint64, string) bool {
	return false
}

// TestHandler_ServeHTTP_daemonErrorsThroughRealFileService drives the real
// daemon.FileService with canned daemon replies, so the whole chain
// daemon text -> FileService -> handler -> responder is exercised the way the
// reported request (/baseq2/server.cfg on a server without that file) is.
func TestHandler_ServeHTTP_daemonErrorsThroughRealFileService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		daemonError    string
		expectedStatus int
		wantError      string
	}{
		{
			name:           "missing_file_is_404",
			daemonError:    "lstatat servers/test1/baseq2/server.cfg: no such file or directory",
			expectedStatus: http.StatusNotFound,
			wantError:      "failed to get file info: file not found",
		},
		{
			name:           "unreadable_file_is_403",
			daemonError:    "lstatat servers/test1/baseq2/server.cfg: permission denied",
			expectedStatus: http.StatusForbidden,
			wantError:      "failed to get file info: permission denied",
		},
		{
			name:           "unknown_daemon_text_is_500",
			daemonError:    "daemon exploded",
			expectedStatus: http.StatusInternalServerError,
			wantError:      "Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			serverRepo := inmemory.NewServerRepository()
			nodeRepo := inmemory.NewNodeRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)

			now := time.Now()
			server := &domain.Server{
				ID:         1,
				UID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
				UUIDShort:  "short1",
				Enabled:    true,
				Installed:  1,
				Name:       "Quake 2",
				GameID:     "q2",
				DSID:       1,
				GameModID:  1,
				ServerIP:   "127.0.0.1",
				ServerPort: 27910,
				Dir:        "servers/test1",
				CreatedAt:  &now,
				UpdatedAt:  &now,
			}
			require.NoError(t, serverRepo.Save(context.Background(), server))
			serverRepo.AddUserServer(1, 1)
			allowUserFilesAbility(t, rbacRepo, 1, 1)

			node := testNode
			require.NoError(t, nodeRepo.Save(context.Background(), &node))

			gateway := &statOnlyGateway{
				statResp: &proto.FileOperationResponse{Success: false, Error: tt.daemonError},
			}
			fileService := daemon.NewFileService(
				gateway, alwaysConnected{}, nil, nil, nil, slog.New(slog.DiscardHandler),
			)
			handler := NewHandler(serverRepo, nodeRepo, rbacService, fileService, api.NewResponder())

			session := &auth.Session{Login: "testuser", Email: "test@example.com", User: &testUser1}
			ctx := auth.ContextWithSession(context.Background(), session)

			req := httptest.NewRequest(
				http.MethodGet, "/api/file-manager/1/stream-file?disk=server&path=%2Fbaseq2%2Fserver.cfg", nil,
			).WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"server": "1"})
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Equal(t, "servers/test1/baseq2/server.cfg", gateway.statPath, "daemon must get the work-relative path")

			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, "error", body["status"])
			assert.Equal(t, tt.wantError, body["error"])
			assert.Equal(t, float64(tt.expectedStatus), body["http_code"])
			assert.NotContains(t, w.Body.String(), "servers/test1", "response must not leak node paths")
			assert.NotContains(t, w.Body.String(), "lstatat", "response must not leak raw daemon text")
		})
	}
}
