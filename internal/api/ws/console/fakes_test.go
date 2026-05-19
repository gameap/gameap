package console

import (
	"context"
	"sync/atomic"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/grpc/session"
	"github.com/gameap/gameap/pkg/proto"
)

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

// newTestServer builds the minimal *domain.Server required by gRPC console
// tests.
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

// newTestNode builds a *domain.Node. script controls the optional
// ScriptGetConsole field; nil means the field is left unset.
func newTestNode(script *string) *domain.Node {
	return &domain.Node{
		ID:               7,
		Name:             "n1",
		WorkPath:         "/srv/gameap",
		ScriptGetConsole: script,
	}
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

var _ session.Stream = (*fakeRegistryStream)(nil)
