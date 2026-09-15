package console

import (
	"context"
	"sync"
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

// recordingRegistryStream satisfies session.Stream and records every command
// pushed toward the daemon, so a test can assert the read-only console endpoint
// never dispatches one.
type recordingRegistryStream struct {
	ctx context.Context //nolint:containedctx // test stub for the session.Stream interface

	mu       sync.Mutex
	commands []*proto.CommandRequest
}

func newRecordingRegistryStream() *recordingRegistryStream {
	return &recordingRegistryStream{ctx: context.Background()}
}

func (s *recordingRegistryStream) Send(msg *proto.GatewayMessage) error {
	if cmd := msg.GetCommand(); cmd != nil {
		s.mu.Lock()
		s.commands = append(s.commands, cmd)
		s.mu.Unlock()
	}

	return nil
}

func (s *recordingRegistryStream) Recv() (*proto.DaemonMessage, error) {
	<-s.ctx.Done()

	return nil, s.ctx.Err()
}

func (s *recordingRegistryStream) Context() context.Context { return s.ctx }

func (s *recordingRegistryStream) commandCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.commands)
}

var _ session.Stream = (*recordingRegistryStream)(nil)
