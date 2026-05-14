package attach

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/grpc/handlers"
	"github.com/gameap/gameap/internal/grpc/session"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

// silentLogger returns a logger that discards everything so test output stays clean.
func silentLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// ----- fakes -----

// fakeRBAC mimics the base.RBAC contract used by AbilityChecker. The two
// gates exposed are isAdmin (for the AbilityNameAdminRolesPermissions check
// inside AbilityChecker) and a per-ability allow set used by CanForEntity.
// If allowed is nil, all CanForEntity checks return true.
type fakeRBAC struct {
	mu      sync.Mutex
	isAdmin bool
	allowed map[domain.AbilityName]bool
	err     error
}

// allowAbility marks one entity-scoped ability as granted.
func (f *fakeRBAC) allowAbility(name domain.AbilityName) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.allowed == nil {
		f.allowed = make(map[domain.AbilityName]bool)
	}
	f.allowed[name] = true
}

// denyAbility removes a single ability from the allow set.
func (f *fakeRBAC) denyAbility(name domain.AbilityName) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.allowed == nil {
		return
	}
	delete(f.allowed, name)
}

func (f *fakeRBAC) Can(_ context.Context, _ uint, abilities []domain.AbilityName) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	if slices.Contains(abilities, domain.AbilityNameAdminRolesPermissions) {
		return f.isAdmin, nil
	}

	return false, nil
}

func (f *fakeRBAC) CanOneOf(_ context.Context, _ uint, _ []domain.AbilityName) (bool, error) {
	return false, nil
}

func (f *fakeRBAC) CanForEntity(
	_ context.Context, _ uint, _ domain.EntityType, _ uint, abilities []domain.AbilityName,
) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	if f.allowed == nil {
		return true, nil
	}
	for _, a := range abilities {
		if !f.allowed[a] {
			return false, nil
		}
	}

	return true, nil
}

func (f *fakeRBAC) GetRoles(_ context.Context, _ uint) ([]string, error) { return nil, nil }
func (f *fakeRBAC) SetRolesToUser(_ context.Context, _ uint, _ []string) error {
	return nil
}

func (f *fakeRBAC) AllowUserAbilitiesForEntity(
	_ context.Context, _ uint, _ uint, _ domain.EntityType, _ []domain.AbilityName,
) error {
	return nil
}

func (f *fakeRBAC) RevokeOrForbidUserAbilitiesForEntity(
	_ context.Context, _ uint, _ uint, _ domain.EntityType, _ []domain.AbilityName,
) error {
	return nil
}

// fakeResponder records WriteError / Write calls so handler-level error paths
// can be asserted without parsing the recorded HTTP body.
type fakeResponder struct {
	mu     sync.Mutex
	errors []error
}

func (r *fakeResponder) WriteError(_ context.Context, rw http.ResponseWriter, err error) {
	r.mu.Lock()
	r.errors = append(r.errors, err)
	r.mu.Unlock()

	type httpError interface{ HTTPStatus() int }
	status := http.StatusInternalServerError
	if he, ok := err.(httpError); ok {
		status = he.HTTPStatus()
	}
	rw.WriteHeader(status)
	_, _ = rw.Write([]byte(err.Error()))
}

func (r *fakeResponder) Write(_ context.Context, rw http.ResponseWriter, _ any) {
	rw.WriteHeader(http.StatusOK)
}

func (r *fakeResponder) errorList() []error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.errors)
}

// fakeDaemonCommands records ExecuteCommand calls and returns the configured
// result/error.
type fakeDaemonCommands struct {
	mu     sync.Mutex
	calls  []daemonCommandCall
	result *daemon.CommandResult
	err    error
}

type daemonCommandCall struct {
	NodeID  uint
	Command string
}

func (f *fakeDaemonCommands) ExecuteCommand(
	_ context.Context, node *domain.Node, command string, _ ...daemon.CommandServiceOption,
) (*daemon.CommandResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, daemonCommandCall{NodeID: node.ID, Command: command})
	if f.err != nil {
		return nil, f.err
	}
	if f.result == nil {
		return &daemon.CommandResult{}, nil
	}

	return f.result, nil
}

func (f *fakeDaemonCommands) executedCommands() []daemonCommandCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.calls)
}

// fakeFileService records Download/Upload calls and returns scripted bytes.
type fakeFileService struct {
	mu            sync.Mutex
	downloadResp  []byte
	downloadErr   error
	uploadErr     error
	downloads     []fileCall
	uploads       []fileCall
	downloadHook  func(call int) ([]byte, error)
	downloadCalls int
}

type fileCall struct {
	NodeID uint
	Path   string
	Data   []byte
	Perms  os.FileMode
}

func (f *fakeFileService) Download(_ context.Context, node *domain.Node, filePath string) ([]byte, error) {
	f.mu.Lock()
	f.downloads = append(f.downloads, fileCall{NodeID: node.ID, Path: filePath})
	idx := f.downloadCalls
	f.downloadCalls++
	hook := f.downloadHook
	defaultResp := f.downloadResp
	defaultErr := f.downloadErr
	f.mu.Unlock()

	if hook != nil {
		return hook(idx)
	}

	return defaultResp, defaultErr
}

func (f *fakeFileService) Upload(
	_ context.Context, node *domain.Node, filePath string,
	content []byte, perms os.FileMode, _ daemon.OwnerOptions,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploads = append(f.uploads, fileCall{NodeID: node.ID, Path: filePath, Data: content, Perms: perms})

	return f.uploadErr
}

func (f *fakeFileService) downloadList() []fileCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.downloads)
}

func (f *fakeFileService) uploadList() []fileCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.uploads)
}

// stubStream is a minimal session.Stream that records sent gateway messages
// and lets the test resolve Recv on demand. Mirrors the helpers_test.go in
// internal/grpc/session.
type stubStream struct {
	ctx     context.Context
	mu      sync.Mutex
	sent    []*proto.GatewayMessage
	sendErr error
	recv    chan *proto.DaemonMessage
}

func newStubStream(ctx context.Context) *stubStream {
	return &stubStream{ctx: ctx, recv: make(chan *proto.DaemonMessage, 4)}
}

func (s *stubStream) Send(m *proto.GatewayMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sendErr != nil {
		return s.sendErr
	}
	s.sent = append(s.sent, m)

	return nil
}

func (s *stubStream) Recv() (*proto.DaemonMessage, error) {
	msg, ok := <-s.recv
	if !ok {
		return nil, context.Canceled
	}

	return msg, nil
}

func (s *stubStream) Context() context.Context { return s.ctx }

func (s *stubStream) sentMessages() []*proto.GatewayMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.sent)
}

// ----- environment fixture -----

// attachEnv collects every collaborator that the tests need to drive the
// handler through a real WebSocket connection.
type attachEnv struct {
	server     *domain.Server
	node       *domain.Node
	user       *domain.User
	rbac       *fakeRBAC
	responder  *fakeResponder
	pubsub     *memory.Memory
	hub        *ws.Hub
	registry   *session.Registry
	attachH    *handlers.AttachHandler
	stream     *stubStream
	dCmds      *fakeDaemonCommands
	files      *fakeFileService
	httpSrv    *httptest.Server
	wsURL      string
	clientConn *websocket.Conn
	nodeRepo   *inmemory.NodeRepository
}

// newAttachEnv builds the fixture and registers a real local session for the
// node so IsConnectedAnywhere returns true. The grpcMode flag controls whether
// a session is registered; legacy tests pass false.
func newAttachEnv(t *testing.T, grpcMode bool) *attachEnv {
	t.Helper()

	const (
		userID   uint = 11
		serverID uint = 33
		nodeID   uint = 7
	)

	user := &domain.User{ID: userID}
	node := &domain.Node{
		ID:       nodeID,
		Enabled:  true,
		Name:     "test-node",
		WorkPath: "/srv/games",
	}
	server := &domain.Server{
		ID:         serverID,
		Enabled:    true,
		Name:       "test-server",
		GameID:     "csgo",
		DSID:       nodeID,
		ServerIP:   "127.0.0.1",
		ServerPort: 27015,
		Dir:        "/srv/games/server33",
		UID:        uuid.New(),
	}
	server.Hydrate()

	serverRepo := inmemory.NewServerRepository()
	require.NoError(t, serverRepo.Save(t.Context(), server))
	serverRepo.AddUserServer(userID, server.ID)

	nodeRepo := inmemory.NewNodeRepository()
	require.NoError(t, nodeRepo.Save(t.Context(), node))

	rbac := &fakeRBAC{isAdmin: true}

	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := session.NewRegistry(ps, "instance-test", silentLogger())
	attachH := handlers.NewAttachHandler(ps, silentLogger())

	stream := newStubStream(t.Context())
	if grpcMode {
		sess := session.NewSession(uint64(nodeID), stream, "1.0.0", nil, func() {})
		require.NoError(t, registry.Register(t.Context(), sess))
	}

	hub := ws.NewHub(silentLogger())
	bridge := ws.NewBridge(hub, ps, silentLogger())
	require.NoError(t, bridge.Start(t.Context()))
	dCmds := &fakeDaemonCommands{}
	files := &fakeFileService{}
	responder := &fakeResponder{}

	h := NewHandler(
		serverRepo, nodeRepo, rbac, hub, nil, registry, attachH,
		dCmds, files, responder,
	)
	// Replace logger with silent one so background goroutines don't spam test output.
	h.logger = silentLogger()

	mr := mux.NewRouter()
	mr.Handle("/api/ws/servers/{server}/attach", authInjector(user, h)).Methods(http.MethodGet)

	httpSrv := httptest.NewServer(mr)
	t.Cleanup(httpSrv.Close)

	env := &attachEnv{
		server:    server,
		node:      node,
		user:      user,
		rbac:      rbac,
		responder: responder,
		pubsub:    ps,
		hub:       hub,
		registry:  registry,
		attachH:   attachH,
		stream:    stream,
		dCmds:     dCmds,
		files:     files,
		httpSrv:   httpSrv,
		nodeRepo:  nodeRepo,
		wsURL: "ws" + strings.TrimPrefix(httpSrv.URL, "http") +
			"/api/ws/servers/33/attach",
	}

	return env
}

// dial opens a real WebSocket connection from the test side to the handler.
// The returned conn is closed automatically at test cleanup. The client read
// limit is bumped to 1 MiB so tests can verify large frames (history payloads)
// without hitting the coder/websocket default of 32 KiB.
func (e *attachEnv) dial(t *testing.T) *websocket.Conn {
	t.Helper()

	dialCtx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(dialCtx, e.wsURL, nil)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	conn.SetReadLimit(1 << 20)
	t.Cleanup(func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})

	e.clientConn = conn

	return conn
}

// authInjector installs a Session in the request context so the handler sees
// an authenticated user, then forwards to next.
func authInjector(user *domain.User, next http.Handler) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		s := &auth.Session{User: user}
		next.ServeHTTP(rw, r.WithContext(auth.ContextWithSession(r.Context(), s)))
	}
}

// publishPubsub publishes a message into the in-memory pubsub.
func (e *attachEnv) publishPubsub(t *testing.T, channel string, msg *pubsub.Message) {
	t.Helper()
	require.NoError(t, e.pubsub.Publish(t.Context(), channel, msg))
}

// setNodeScriptSendCommand mutates the node and re-saves it so the handler's
// node lookup returns the updated script_send_command.
func (e *attachEnv) setNodeScriptSendCommand(t *testing.T, cmd string) {
	t.Helper()
	e.node.ScriptSendCommand = &cmd
	require.NoError(t, e.nodeRepo.Save(t.Context(), e.node))
}

// setNodeScriptGetConsole mutates the node and re-saves it so the handler's
// node lookup returns the updated script_get_console.
func (e *attachEnv) setNodeScriptGetConsole(t *testing.T, cmd string) {
	t.Helper()
	e.node.ScriptGetConsole = &cmd
	require.NoError(t, e.nodeRepo.Save(t.Context(), e.node))
}

// ----- frame helpers -----

type wsFrame struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	Error   string          `json:"error"`
	Ts      int64           `json:"ts"`
}

func readFrame(t *testing.T, c *websocket.Conn, timeout time.Duration) (wsFrame, bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, data, err := c.Read(ctx)
	if err != nil {
		return wsFrame{}, false
	}

	var f wsFrame
	require.NoError(t, json.Unmarshal(data, &f))

	return f, true
}

// readFrameOfType reads frames until the requested type appears or timeout
// expires, returning the matching frame. Frames of other types are discarded.
// Useful when the handler may emit unrelated frames first (e.g. attach.started)
// before the frame the test cares about.
func readFrameOfType(t *testing.T, c *websocket.Conn, want string, timeout time.Duration) (wsFrame, bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		left := time.Until(deadline)
		if left <= 0 {
			break
		}
		f, ok := readFrame(t, c, left)
		if !ok {
			return wsFrame{}, false
		}
		if f.Type == want {
			return f, true
		}
	}

	return wsFrame{}, false
}

// writeJSONFrame marshals an InboundMessage and writes it as a text frame.
func writeJSONFrame(t *testing.T, c *websocket.Conn, msg *ws.InboundMessage) {
	t.Helper()

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	require.NoError(t, c.Write(t.Context(), websocket.MessageText, data))
}

// rawPayload helps build inbound messages with json-encoded payloads.
func rawPayload(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)

	return b
}
