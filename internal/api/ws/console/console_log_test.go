package console

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/grpc/session"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCatScript is a placeholder shell command shared across tests that need
// node.ScriptGetConsole to be non-empty without caring about the contents.
const testCatScript = "cat /tmp/console.txt"

// silentLogger returns a logger that discards everything.
func silentLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// fakeConsoleLogService records the parameters of each call so a test can
// assert the handler forwards the right node/server pair.
type fakeConsoleLogService struct {
	result string
	err    error

	calls atomic.Int32
}

func (f *fakeConsoleLogService) GetConsoleLog(
	_ context.Context, _ uint64, _ uint64, _ int64,
) (string, error) {
	f.calls.Add(1)

	return f.result, f.err
}

// fakeFileService scripts Download/Upload with optional error returns and
// tracks the requested paths.
type fakeFileService struct {
	downloadResult []byte
	downloadErr    error
	downloadPath   string

	uploadErr     error
	uploadCalls   atomic.Int32
	uploadPath    string
	uploadContent []byte
}

func (f *fakeFileService) Download(
	_ context.Context, _ *domain.Node, filePath string,
) ([]byte, error) {
	f.downloadPath = filePath

	return f.downloadResult, f.downloadErr
}

func (f *fakeFileService) Upload(
	_ context.Context, _ *domain.Node, filePath string,
	content []byte, _ os.FileMode, _ daemon.OwnerOptions,
) error {
	f.uploadCalls.Add(1)
	f.uploadPath = filePath
	f.uploadContent = content

	return f.uploadErr
}

// fakeDaemonCommands scripts ExecuteCommand and records the command string
// passed to it so tests can verify shortcode replacement.
type fakeDaemonCommands struct {
	result *daemon.CommandResult
	err    error

	calls   atomic.Int32
	lastCmd string
}

func (f *fakeDaemonCommands) ExecuteCommand(
	_ context.Context, _ *domain.Node, command string, _ ...daemon.CommandServiceOption,
) (*daemon.CommandResult, error) {
	f.calls.Add(1)
	f.lastCmd = command

	return f.result, f.err
}

// newTestServer builds the minimal *domain.Server required by getConsoleLog
// and downloadOutputFile.
func newTestServer() *domain.Server {
	return &domain.Server{
		ID:         42,
		DSID:       7,
		ServerIP:   "127.0.0.1",
		ServerPort: 27015,
		GameID:     "cs",
		Dir:        "/srv/gs/test",
	}
}

// newTestNode builds a minimal *domain.Node. Pass an optional script for the
// "get console" path; nil means the field is left unset.
func newTestNode(script *string) *domain.Node {
	return &domain.Node{
		ID:               7,
		Name:             "n1",
		WorkPath:         "/srv/gameap",
		ScriptGetConsole: script,
	}
}

// newHandlerForLog assembles a Handler with only the dependencies required by
// getConsoleLog: a real (empty) session.Registry built on memory pubsub,
// plus the supplied collaborators.
func newHandlerForLog(
	t *testing.T,
	cls consoleLogService,
	dc daemonCommands,
	fs fileService,
) *Handler {
	t.Helper()

	mem := memory.New()
	t.Cleanup(func() { _ = mem.Close() })

	registry := session.NewRegistry(mem, "test-instance", silentLogger())

	return &Handler{
		registry:          registry,
		daemonCommands:    dc,
		fileService:       fs,
		consoleLogService: cls,
		logger:            silentLogger(),
	}
}

// ---------- getConsoleLog ----------

func TestHandler_GetConsoleLog(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T) (*Handler, *domain.Server, *domain.Node, *fakeConsoleLogService, *fakeDaemonCommands, *fakeFileService)
		want         string
		wantError    string
		wantCLSCalls int32
		wantDCCalls  int32
		wantFSPath   string
	}{
		{
			name: "service_returns_value_no_fallback_to_script_or_file",
			setup: func(t *testing.T) (*Handler, *domain.Server, *domain.Node, *fakeConsoleLogService, *fakeDaemonCommands, *fakeFileService) {
				t.Helper()

				cls := &fakeConsoleLogService{result: "from-service"}
				script := testCatScript
				dc := &fakeDaemonCommands{result: &daemon.CommandResult{Output: "from-script"}}
				fs := &fakeFileService{downloadResult: []byte("from-file")}
				h := newHandlerForLog(t, cls, dc, fs)

				return h, newTestServer(), newTestNode(&script), cls, dc, fs
			},
			want:         "from-service",
			wantCLSCalls: 1,
			wantDCCalls:  0,
		},
		{
			name: "service_error_falls_back_to_script",
			setup: func(t *testing.T) (*Handler, *domain.Server, *domain.Node, *fakeConsoleLogService, *fakeDaemonCommands, *fakeFileService) {
				t.Helper()

				cls := &fakeConsoleLogService{err: errors.New("service unavailable")}
				script := testCatScript
				dc := &fakeDaemonCommands{result: &daemon.CommandResult{Output: "from-script"}}
				fs := &fakeFileService{}
				h := newHandlerForLog(t, cls, dc, fs)

				return h, newTestServer(), newTestNode(&script), cls, dc, fs
			},
			want:         "from-script",
			wantCLSCalls: 1,
			wantDCCalls:  1,
		},
		{
			name: "service_unset_uses_script_when_present",
			setup: func(t *testing.T) (*Handler, *domain.Server, *domain.Node, *fakeConsoleLogService, *fakeDaemonCommands, *fakeFileService) {
				t.Helper()

				script := testCatScript
				dc := &fakeDaemonCommands{result: &daemon.CommandResult{Output: "scripted"}}
				fs := &fakeFileService{}
				h := newHandlerForLog(t, nil, dc, fs)

				return h, newTestServer(), newTestNode(&script), nil, dc, fs
			},
			want:         "scripted",
			wantCLSCalls: 0,
			wantDCCalls:  1,
		},
		{
			name: "script_replaces_server_shortcodes_before_executing",
			setup: func(t *testing.T) (*Handler, *domain.Server, *domain.Node, *fakeConsoleLogService, *fakeDaemonCommands, *fakeFileService) {
				t.Helper()

				script := "tail -n 100 {dir}/output.log"
				dc := &fakeDaemonCommands{result: &daemon.CommandResult{Output: "ok"}}
				fs := &fakeFileService{}
				h := newHandlerForLog(t, nil, dc, fs)

				return h, newTestServer(), newTestNode(&script), nil, dc, fs
			},
			want:         "ok",
			wantCLSCalls: 0,
			wantDCCalls:  1,
		},
		{
			name: "script_execution_error_propagates_with_wrap",
			setup: func(t *testing.T) (*Handler, *domain.Server, *domain.Node, *fakeConsoleLogService, *fakeDaemonCommands, *fakeFileService) {
				t.Helper()

				script := testCatScript
				dc := &fakeDaemonCommands{err: errors.New("daemon down")}
				fs := &fakeFileService{}
				h := newHandlerForLog(t, nil, dc, fs)

				return h, newTestServer(), newTestNode(&script), nil, dc, fs
			},
			wantError:    "failed to execute get console script: daemon down",
			wantCLSCalls: 0,
			wantDCCalls:  1,
		},
		{
			name: "no_service_no_script_no_grpc_falls_back_to_file_download",
			setup: func(t *testing.T) (*Handler, *domain.Server, *domain.Node, *fakeConsoleLogService, *fakeDaemonCommands, *fakeFileService) {
				t.Helper()

				dc := &fakeDaemonCommands{}
				fs := &fakeFileService{downloadResult: []byte("from-file\nlast line\n")}
				h := newHandlerForLog(t, nil, dc, fs)

				return h, newTestServer(), newTestNode(nil), nil, dc, fs
			},
			want:         "from-file\nlast line\n",
			wantCLSCalls: 0,
			wantDCCalls:  0,
			wantFSPath:   "/srv/gs/test/output.txt",
		},
		{
			name: "empty_script_string_treated_as_no_script_falls_back_to_file",
			setup: func(t *testing.T) (*Handler, *domain.Server, *domain.Node, *fakeConsoleLogService, *fakeDaemonCommands, *fakeFileService) {
				t.Helper()

				empty := ""
				dc := &fakeDaemonCommands{}
				fs := &fakeFileService{downloadResult: []byte("file-content")}
				h := newHandlerForLog(t, nil, dc, fs)

				return h, newTestServer(), newTestNode(&empty), nil, dc, fs
			},
			want:         "file-content",
			wantCLSCalls: 0,
			wantDCCalls:  0,
			wantFSPath:   "/srv/gs/test/output.txt",
		},
		{
			name: "download_error_propagates_with_wrap",
			setup: func(t *testing.T) (*Handler, *domain.Server, *domain.Node, *fakeConsoleLogService, *fakeDaemonCommands, *fakeFileService) {
				t.Helper()

				dc := &fakeDaemonCommands{}
				fs := &fakeFileService{downloadErr: errors.New("connection refused")}
				h := newHandlerForLog(t, nil, dc, fs)

				return h, newTestServer(), newTestNode(nil), nil, dc, fs
			},
			wantError: "failed to download console log: connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			h, server, node, cls, dc, fs := tt.setup(t)

			// ACT
			got, err := h.getConsoleLog(context.Background(), server, node)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got, "console output should match expected value")
			}

			if cls != nil {
				assert.Equal(t, tt.wantCLSCalls, cls.calls.Load(),
					"consoleLogService.GetConsoleLog call count mismatch")
			}
			assert.Equal(t, tt.wantDCCalls, dc.calls.Load(),
				"daemonCommands.ExecuteCommand call count mismatch")

			if tt.wantFSPath != "" {
				assert.Equal(t, tt.wantFSPath, fs.downloadPath,
					"fileService.Download path mismatch")
			}
		})
	}
}

func TestHandler_GetConsoleLog_grpcConnectedSkipsDownload(t *testing.T) {
	// ARRANGE — register a session for node 7 so registry.IsConnected returns true.
	mem := memory.New()
	t.Cleanup(func() { _ = mem.Close() })

	registry := session.NewRegistry(mem, "test-instance", silentLogger())
	sess := session.NewSession(7, newFakeRegistryStream(), "1.0", nil, func() {})
	require.NoError(t, registry.Register(context.Background(), sess))

	fs := &fakeFileService{downloadResult: []byte("must-not-be-returned")}
	h := &Handler{
		registry:    registry,
		fileService: fs,
		logger:      silentLogger(),
	}

	server := newTestServer()
	node := newTestNode(nil)

	// ACT — node has no script and no consoleLogService, but the daemon is
	// connected via gRPC, so getConsoleLog must short-circuit before touching
	// the file service.
	got, err := h.getConsoleLog(context.Background(), server, node)

	// ASSERT
	require.NoError(t, err)
	assert.Empty(t, got, "with gRPC connected the legacy path must not run")
	assert.Empty(t, fs.downloadPath, "file service must not be invoked when gRPC is connected")
}

// ---------- downloadOutputFile ----------

func TestHandler_DownloadOutputFile(t *testing.T) {
	const maxSymbols = 65536

	tests := []struct {
		name      string
		dlBytes   []byte
		dlErr     error
		serverDir string
		want      string
		wantLen   int // when set, asserts len(got) == wantLen
		wantPath  string
		wantError string
	}{
		{
			name:      "small_payload_returned_as_is",
			dlBytes:   []byte("short content"),
			serverDir: "/srv/gs/a",
			want:      "short content",
			wantPath:  "/srv/gs/a/output.txt",
		},
		{
			name:      "exactly_64KB_returned_unchanged",
			dlBytes:   bytesOf('x', maxSymbols),
			serverDir: "/srv/gs/b",
			wantLen:   maxSymbols,
			wantPath:  "/srv/gs/b/output.txt",
		},
		{
			name:      "oversized_payload_truncated_to_last_64KB",
			dlBytes:   appendBytes([]byte("HEADER-IGNORED-PREFIX"), bytesOf('y', maxSymbols)),
			serverDir: "/srv/gs/c",
			wantLen:   maxSymbols,
			wantPath:  "/srv/gs/c/output.txt",
		},
		{
			name:      "empty_payload_returns_empty_string",
			dlBytes:   nil,
			serverDir: "/srv/gs/d",
			want:      "",
			wantPath:  "/srv/gs/d/output.txt",
		},
		{
			name:      "download_error_wrapped",
			dlErr:     errors.New("not found"),
			serverDir: "/srv/gs/e",
			wantError: "failed to download console log: not found",
			wantPath:  "/srv/gs/e/output.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			fs := &fakeFileService{
				downloadResult: tt.dlBytes,
				downloadErr:    tt.dlErr,
			}
			h := &Handler{
				fileService: fs,
				logger:      silentLogger(),
			}
			node := newTestNode(nil)

			// ACT
			got, err := h.downloadOutputFile(context.Background(), node, tt.serverDir)

			// ASSERT
			assertDownloadResult(t, tt.wantError, tt.wantLen, tt.want, tt.dlBytes, maxSymbols, got, err)
			assert.Equal(t, tt.wantPath, fs.downloadPath, "download path must equal serverDir/output.txt")
		})
	}
}

// assertDownloadResult inverts the nested if-else-if cascade that would
// otherwise live inside the table loop body for downloadOutputFile cases.
func assertDownloadResult(
	t *testing.T,
	wantError string,
	wantLen int,
	want string,
	rawInput []byte,
	maxSymbols int,
	got string,
	err error,
) {
	t.Helper()

	if wantError != "" {
		require.Error(t, err)
		assert.Contains(t, err.Error(), wantError)

		return
	}

	require.NoError(t, err)

	if wantLen == 0 {
		assert.Equal(t, want, got)

		return
	}

	assert.Len(t, got, wantLen, "truncated payload length mismatch")
	if len(rawInput) > maxSymbols {
		assert.Equal(t, string(rawInput[len(rawInput)-maxSymbols:]), got,
			"truncated content must be the suffix of the input")
	}
}

// ---------- canSendCommands ----------

func TestHandler_CanSendCommands_returnsFalseWhenAbilityCheckFails(t *testing.T) {
	// ARRANGE — an AbilityChecker built with a nil RBAC backend will panic on
	// any call, so we wrap it with a denying RBAC implementation.
	h := &Handler{
		abilityChecker: newAbilityCheckerWithRBAC(&denyAllRBAC{}),
		logger:         silentLogger(),
	}
	user := &domain.User{ID: 1}
	server := newTestServer()

	// ACT
	got := h.canSendCommands(context.Background(), user, server)

	// ASSERT
	assert.False(t, got, "denied ability must return false")
}

func TestHandler_CanSendCommands_returnsTrueWhenAdmin(t *testing.T) {
	// ARRANGE
	h := &Handler{
		abilityChecker: newAbilityCheckerWithRBAC(&allowAllRBAC{}),
		logger:         silentLogger(),
	}
	user := &domain.User{ID: 1}
	server := newTestServer()

	// ACT
	got := h.canSendCommands(context.Background(), user, server)

	// ASSERT
	assert.True(t, got)
}

// ---------- helpers ----------

func bytesOf(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}

	return out
}

func appendBytes(parts ...[]byte) []byte {
	total := 0
	for _, p := range parts {
		total += len(p)
	}
	out := make([]byte, 0, total)
	for _, p := range parts {
		out = append(out, p...)
	}

	return out
}

// fakeRegistryStream satisfies session.Stream with no-op behaviour. Used to
// register a fake session against the registry so IsConnected returns true.
type fakeRegistryStream struct {
	ctx context.Context //nolint:containedctx // test stub for the session.Stream interface
}

func newFakeRegistryStream() *fakeRegistryStream {
	return &fakeRegistryStream{ctx: context.Background()}
}

func (s *fakeRegistryStream) Send(_ *proto.GatewayMessage) error { return nil }

func (s *fakeRegistryStream) Recv() (*proto.DaemonMessage, error) {
	<-s.ctx.Done()

	return nil, s.ctx.Err()
}

func (s *fakeRegistryStream) Context() context.Context { return s.ctx }

// Compile-time check.
var _ session.Stream = (*fakeRegistryStream)(nil)
