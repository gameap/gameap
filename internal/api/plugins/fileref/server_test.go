package fileref_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/api/plugins/fileref"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *auditCapture) snapshot() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]audit.Event(nil), a.events...)
}

type fakeChecker struct {
	allowed bool
	err     error
}

func (f fakeChecker) Has(_ context.Context, pluginID uint64, _ domain.PluginPermission) (bool, error) {
	if pluginID == 0 {
		return false, nil
	}

	return f.allowed, f.err
}

type closeRecorder struct {
	io.Reader

	closed bool
}

func (c *closeRecorder) Close() error {
	c.closed = true

	return nil
}

type fakeFileService struct {
	mu         sync.Mutex
	info       *daemon.FileDetails
	infoErr    error
	content    string
	streamErr  error
	infoCalls  int
	streamOpen *closeRecorder
	reader     io.Reader
}

func (f *fakeFileService) GetFileInfo(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.infoCalls++

	if f.infoErr != nil {
		return nil, f.infoErr
	}

	return f.info, nil
}

func (f *fakeFileService) DownloadStream(_ context.Context, _ *domain.Node, _ string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.streamErr != nil {
		return nil, f.streamErr
	}

	reader := f.reader
	if reader == nil {
		reader = strings.NewReader(f.content)
	}

	f.streamOpen = &closeRecorder{Reader: reader}

	return f.streamOpen, nil
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("daemon connection lost")
}

const testNodeID = 1

func newTestServer(t *testing.T, files *fakeFileService, checker fakeChecker, recorder *auditCapture) *fileref.Server {
	t.Helper()

	nodes := inmemory.NewNodeRepository()
	require.NoError(t, nodes.Save(context.Background(), &domain.Node{Name: "node-1", OS: domain.NodeOSLinux, WorkPath: "/srv/gameap"}))

	var auditLogger audit.Logger
	if recorder != nil {
		auditLogger = recorder
	}

	return fileref.NewServer(files, nodes, checker, auditLogger)
}

func fileRequest(ref *proto.FileRef, headers map[string]string, status int) pkgplugin.FileRefRequest {
	return pkgplugin.FileRefRequest{
		PluginID:   42,
		PluginName: "exporter",
		Ref:        ref,
		Headers:    headers,
		StatusCode: status,
	}
}

func csvFile() *daemon.FileDetails {
	return &daemon.FileDetails{Name: "report.csv", Size: 5, Type: daemon.FileTypeFile}
}

func TestServeFileRef_streams_attachment(t *testing.T) {
	t.Parallel()
	files := &fakeFileService{info: csvFile(), content: "a,b\n1"}
	server := newTestServer(t, files, fakeChecker{allowed: true}, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/plugins/exporter/report", nil)

	err := server.ServeFileRef(w, req, fileRequest(
		&proto.FileRef{NodeId: testNodeID, Path: "/srv/gameap/servers/cs2/report.csv", Filename: "report.csv"},
		map[string]string{"Content-Type": "text/csv; charset=utf-8", "X-Plugin": "exporter"},
		0,
	))
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "a,b\n1", w.Body.String())
	assert.Equal(t, "text/csv; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename=report.csv; filename*=UTF-8''report.csv`, w.Header().Get("Content-Disposition"))
	assert.Equal(t, "5", w.Header().Get("Content-Length"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "sandbox", w.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "none", w.Header().Get("Accept-Ranges"))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, "exporter", w.Header().Get("X-Plugin"), "custom plugin headers pass through")
	require.NotNil(t, files.streamOpen)
	assert.True(t, files.streamOpen.closed)
}

func TestServeFileRef_header_contract(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		ref         *proto.FileRef
		headers     map[string]string
		status      int
		wantStatus  int
		wantHeaders map[string]string
	}{
		{
			name:       "filename_defaults_to_path_base",
			ref:        &proto.FileRef{NodeId: testNodeID, Path: "/srv/gameap/servers/cs2/maps/de_dust2.bsp"},
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Content-Disposition": `attachment; filename=de_dust2.bsp; filename*=UTF-8''de_dust2.bsp`,
				"Content-Type":        "application/octet-stream",
			},
		},
		{
			name:       "windows_path_base",
			ref:        &proto.FileRef{NodeId: testNodeID, Path: `C:\servers\cs2\report.csv`},
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Content-Disposition": `attachment; filename=report.csv; filename*=UTF-8''report.csv`,
			},
		},
		{
			name:        "plugin_cache_control_is_kept",
			ref:         &proto.FileRef{NodeId: testNodeID, Path: "/srv/gameap/a.bin"},
			headers:     map[string]string{"Cache-Control": "private, max-age=60"},
			wantStatus:  http.StatusOK,
			wantHeaders: map[string]string{"Cache-Control": "private, max-age=60"},
		},
		{
			name:        "invalid_content_type_falls_back",
			ref:         &proto.FileRef{NodeId: testNodeID, Path: "/srv/gameap/a.bin"},
			headers:     map[string]string{"Content-Type": "not a type"},
			wantStatus:  http.StatusOK,
			wantHeaders: map[string]string{"Content-Type": "application/octet-stream"},
		},
		{
			name:       "plugin_status_code_is_used",
			ref:        &proto.FileRef{NodeId: testNodeID, Path: "/srv/gameap/a.bin"},
			status:     http.StatusCreated,
			wantStatus: http.StatusCreated,
		},
		{
			name: "reserved_headers_are_overridden",
			ref:  &proto.FileRef{NodeId: testNodeID, Path: "/srv/gameap/a.bin", Filename: "a.bin"},
			headers: map[string]string{
				"Content-Length":      "1",
				"Content-Disposition": "inline",
				"Content-Encoding":    "gzip",
				"Transfer-Encoding":   "chunked",
			},
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				"Content-Length":      "5",
				"Content-Disposition": `attachment; filename=a.bin; filename*=UTF-8''a.bin`,
				"Content-Encoding":    "",
				"Transfer-Encoding":   "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files := &fakeFileService{info: csvFile(), content: "a,b\n1"}
			server := newTestServer(t, files, fakeChecker{allowed: true}, nil)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/plugins/exporter/report", nil)

			err := server.ServeFileRef(w, req, fileRequest(tt.ref, tt.headers, tt.status))
			require.NoError(t, err)

			assert.Equal(t, tt.wantStatus, w.Code)
			for name, want := range tt.wantHeaders {
				assert.Equal(t, want, w.Header().Get(name), name)
			}
		})
	}
}

func TestServeFileRef_refusals(t *testing.T) {
	t.Parallel()
	validRef := &proto.FileRef{NodeId: testNodeID, Path: "/srv/gameap/servers/cs2/report.csv"}

	tests := []struct {
		name        string
		ref         *proto.FileRef
		pluginID    uint64
		checker     fakeChecker
		files       *fakeFileService
		wantStatus  int
		wantError   string
		wantStat    bool
		wantDenied  bool
		unsetPlugin bool
	}{
		{
			name:       "nil_ref",
			ref:        nil,
			checker:    fakeChecker{allowed: true},
			files:      &fakeFileService{info: csvFile()},
			wantStatus: http.StatusBadRequest,
			wantError:  "file reference is required",
		},
		{
			name:       "node_id_zero",
			ref:        &proto.FileRef{Path: "/srv/gameap/a"},
			checker:    fakeChecker{allowed: true},
			files:      &fakeFileService{info: csvFile()},
			wantStatus: http.StatusBadRequest,
			wantError:  "node id is required",
		},
		{
			name:       "empty_path",
			ref:        &proto.FileRef{NodeId: testNodeID},
			checker:    fakeChecker{allowed: true},
			files:      &fakeFileService{info: csvFile()},
			wantStatus: http.StatusBadRequest,
			wantError:  "file path is required",
		},
		{
			name:       "path_traversal",
			ref:        &proto.FileRef{NodeId: testNodeID, Path: "/srv/gameap/../etc/passwd"},
			checker:    fakeChecker{allowed: true},
			files:      &fakeFileService{info: csvFile()},
			wantStatus: http.StatusBadRequest,
			wantError:  "path contains invalid directory traversal",
		},
		{
			name:       "denied_without_files_permission",
			ref:        validRef,
			checker:    fakeChecker{allowed: false},
			files:      &fakeFileService{info: csvFile()},
			wantStatus: http.StatusForbidden,
			wantError:  "plugin permission files required",
			wantDenied: true,
		},
		{
			name:        "transient_plugin_is_denied",
			ref:         validRef,
			unsetPlugin: true,
			checker:     fakeChecker{allowed: true},
			files:       &fakeFileService{info: csvFile()},
			wantStatus:  http.StatusForbidden,
			wantError:   "plugin permission files required",
			wantDenied:  true,
		},
		{
			name:       "permission_check_error",
			ref:        validRef,
			checker:    fakeChecker{err: errors.New("database down")},
			files:      &fakeFileService{info: csvFile()},
			wantStatus: http.StatusInternalServerError,
			wantError:  "failed to check plugin permission",
		},
		{
			name:       "node_not_found",
			ref:        &proto.FileRef{NodeId: 999, Path: "/srv/gameap/a"},
			checker:    fakeChecker{allowed: true},
			files:      &fakeFileService{info: csvFile()},
			wantStatus: http.StatusNotFound,
			wantError:  "node not found",
		},
		{
			name:       "directory_is_rejected",
			ref:        validRef,
			checker:    fakeChecker{allowed: true},
			files:      &fakeFileService{info: &daemon.FileDetails{Name: "cs2", Type: daemon.FileTypeDir}},
			wantStatus: http.StatusBadRequest,
			wantError:  "path is a directory",
			wantStat:   true,
		},
		{
			name:       "stat_error_keeps_daemon_status",
			ref:        validRef,
			checker:    fakeChecker{allowed: true},
			files:      &fakeFileService{infoErr: &daemon.FileError{Op: "stat", Err: daemon.ErrFileNotFound, Detail: "no such file"}},
			wantStatus: http.StatusNotFound,
			wantError:  "file not found",
			wantStat:   true,
		},
		{
			name:       "missing_file_info_is_not_found",
			ref:        validRef,
			checker:    fakeChecker{allowed: true},
			files:      &fakeFileService{},
			wantStatus: http.StatusNotFound,
			wantError:  "file not found",
			wantStat:   true,
		},
		{
			name:       "open_stream_error",
			ref:        validRef,
			checker:    fakeChecker{allowed: true},
			files:      &fakeFileService{info: csvFile(), streamErr: errors.New("daemon offline")},
			wantStatus: http.StatusInternalServerError,
			wantError:  "failed to open file stream",
			wantStat:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recorder := &auditCapture{}
			server := newTestServer(t, tt.files, tt.checker, recorder)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/plugins/exporter/report", nil)

			request := fileRequest(tt.ref, nil, 0)
			if tt.unsetPlugin {
				request.PluginID = 0
			}

			err := server.ServeFileRef(w, req, request)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)

			var withStatus interface{ HTTPStatus() int }
			if tt.wantStatus == http.StatusInternalServerError {
				assert.False(t, errors.As(err, &withStatus), "internal failures carry no status")
			} else {
				require.ErrorAs(t, err, &withStatus)
				assert.Equal(t, tt.wantStatus, withStatus.HTTPStatus())
			}

			assert.Empty(t, w.Body.String(), "nothing is written when the request is refused")
			assert.Equal(t, tt.wantStat, tt.files.infoCalls == 1, "stat happens only after validation and authorization")
			assert.Nil(t, tt.files.streamOpen)

			events := recorder.snapshot()
			if tt.wantDenied {
				require.Len(t, events, 1)
				assert.Equal(t, audit.EventAccessDenied, events[0].Type)
				assert.Equal(t, audit.OutcomeDenied, events[0].Outcome)
				assert.Equal(t, "plugin", events[0].ResourceType)
				assert.Equal(t, "plugin_permission_missing", events[0].Reason)
			} else {
				assert.Empty(t, events)
			}
		})
	}
}

func TestServeFileRef_copy_error_after_headers(t *testing.T) {
	t.Parallel()
	files := &fakeFileService{info: csvFile(), reader: failingReader{}}
	server := newTestServer(t, files, fakeChecker{allowed: true}, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/plugins/exporter/report", nil)

	err := server.ServeFileRef(w, req, fileRequest(&proto.FileRef{NodeId: testNodeID, Path: "/srv/gameap/a.bin"}, nil, 0))

	require.NoError(t, err, "the headers are already out; the broken transfer is only logged")
	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, files.streamOpen)
	assert.True(t, files.streamOpen.closed)
}
