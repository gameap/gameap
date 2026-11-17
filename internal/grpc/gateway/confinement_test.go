package gateway

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The daemon learns the directory a file request is confined to from the
// request itself, so each of them has to carry the root of its context.
func TestService_FileRequestsCarryTheConfinementRoot(t *testing.T) {
	t.Parallel()

	const nodeID uint64 = 1

	requests := []struct {
		name string
		send func(ctx context.Context, svc *Service)
		root func(msg *proto.GatewayMessage) (string, bool)
	}{
		{
			name: "file_read",
			send: func(ctx context.Context, svc *Service) {
				_, _ = svc.RequestFileRead(ctx, nodeID, "servers/tenant-a/server.cfg", 0, 0)
			},
			root: func(msg *proto.GatewayMessage) (string, bool) {
				return msg.GetFileRead().GetRoot(), msg.GetFileRead() != nil
			},
		},
		{
			name: "file_write",
			send: func(ctx context.Context, svc *Service) {
				_ = svc.RequestFileWrite(
					ctx, nodeID, "servers/tenant-a/server.cfg", []byte("x"), 0o644, true, daemon.OwnerOptions{},
				)
			},
			root: func(msg *proto.GatewayMessage) (string, bool) {
				return msg.GetFileWrite().GetRoot(), msg.GetFileWrite() != nil
			},
		},
		{
			name: "file_list",
			send: func(ctx context.Context, svc *Service) {
				_, _ = svc.RequestFileList(ctx, nodeID, "servers/tenant-a", false, "")
			},
			root: func(msg *proto.GatewayMessage) (string, bool) {
				return msg.GetFileList().GetRoot(), msg.GetFileList() != nil
			},
		},
		{
			name: "file_operation",
			send: func(ctx context.Context, svc *Service) {
				_, _ = svc.RequestFileOperation(ctx, nodeID, &proto.FileOperationRequest{
					Operation: proto.FileOperationType_FILE_OPERATION_TYPE_DELETE,
					Parameters: &proto.FileOperationRequest_DeleteParams{
						DeleteParams: &proto.DeleteParams{Path: "servers/tenant-a/old.cfg"},
					},
				})
			},
			root: func(msg *proto.GatewayMessage) (string, bool) {
				return msg.GetFileOperation().GetRoot(), msg.GetFileOperation() != nil
			},
		},
		{
			name: "file_upload_task",
			send: func(ctx context.Context, svc *Service) {
				_ = svc.RequestFileUploadTask(
					ctx, nodeID, "xfer-up", "servers/tenant-a/map.bsp", "checksum", 10, 0o644, daemon.OwnerOptions{},
				)
			},
			root: func(msg *proto.GatewayMessage) (string, bool) {
				return msg.GetFileUploadTask().GetRoot(), msg.GetFileUploadTask() != nil
			},
		},
		{
			name: "file_download_task",
			send: func(ctx context.Context, svc *Service) {
				_ = svc.RequestFileDownloadTask(ctx, nodeID, "xfer-down", "servers/tenant-a/map.bsp")
			},
			root: func(msg *proto.GatewayMessage) (string, bool) {
				return msg.GetFileDownloadTask().GetRoot(), msg.GetFileDownloadTask() != nil
			},
		},
	}

	roots := []struct {
		name string
		root string
	}{
		{name: "confined_to_the_server", root: "servers/tenant-a"},
		{name: "no_root_named", root: ""},
	}

	for _, request := range requests {
		for _, rt := range roots {
			t.Run(request.name+"/"+rt.name, func(t *testing.T) {
				t.Parallel()

				svc, deps := newServiceWithDeps(t)
				stream := connectAndRegisterSession(t, deps)

				// The message goes out before the answer is awaited, so a context
				// that is already done ends the call right after the send.
				ctx, cancel := context.WithCancel(daemon.WithConfinementRoot(context.Background(), rt.root))
				cancel()

				request.send(ctx, svc)

				sent := stream.Sent()
				require.Len(t, sent, 1)

				root, ok := request.root(sent[0])
				require.True(t, ok, "unexpected payload: %T", sent[0].GetPayload())
				assert.Equal(t, rt.root, root)
			})
		}
	}
}

// A caller that already put a root into the operation request (the archive
// service does, for requests that outlive their context) is not overruled.
func TestService_RequestFileOperation_KeepsTheRootOfTheRequest(t *testing.T) {
	t.Parallel()

	svc, deps := newServiceWithDeps(t)
	stream := connectAndRegisterSession(t, deps)

	ctx, cancel := context.WithCancel(daemon.WithConfinementRoot(context.Background(), "servers/tenant-a"))
	cancel()

	_, _ = svc.RequestFileOperation(ctx, 1, &proto.FileOperationRequest{
		Operation: proto.FileOperationType_FILE_OPERATION_TYPE_STAT,
		Parameters: &proto.FileOperationRequest_StatParams{
			StatParams: &proto.StatParams{Path: "servers/tenant-b/server.cfg"},
		},
		Root: "servers/tenant-b",
	})

	sent := stream.Sent()
	require.Len(t, sent, 1)
	require.NotNil(t, sent[0].GetFileOperation())
	assert.Equal(t, "servers/tenant-b", sent[0].GetFileOperation().GetRoot())
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// A daemon that does not advertise file_confinement ignores the root: the node
// stays exposed until the daemon is updated, and the panel log has to say so.
func TestService_Connect_WarnsAboutDaemonWithoutFileConfinement(t *testing.T) {
	t.Parallel()

	const warning = "daemon is outdated: it does not keep file requests inside the game server directory"

	tests := []struct {
		name         string
		capabilities []string
		wantWarning  bool
	}{
		{
			name:         "outdated_daemon",
			capabilities: []string{"file_transfer", "archive"},
			wantWarning:  true,
		},
		{
			name:         "daemon_without_capabilities",
			capabilities: nil,
			wantWarning:  true,
		},
		{
			name:         "daemon_that_confines",
			capabilities: []string{"file_transfer", "archive", daemon.CapabilityFileConfinement},
			wantWarning:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, deps := newServiceWithDeps(t)
			setupAuthorizedNode(t, deps, "secret")

			logs := &syncBuffer{}
			svc.logger = slog.New(slog.NewTextHandler(logs, nil))

			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			stream := newStubConnectServer(ctx)
			stream.PushMessage(&proto.DaemonMessage{
				RequestId: "register-1",
				Payload: &proto.DaemonMessage_Register{
					Register: &proto.RegisterRequest{
						NodeId:       1,
						ApiKey:       "secret",
						Version:      "v4.1.5",
						Capabilities: tt.capabilities,
					},
				},
			})

			connectErrCh := make(chan error, 1)
			go func() {
				connectErrCh <- svc.Connect(stream)
			}()

			require.Eventually(t, func() bool {
				return len(stream.Sent()) > 0
			}, 2*time.Second, 5*time.Millisecond, "RegisterAck must be sent")

			stream.CloseRecv()

			select {
			case err := <-connectErrCh:
				require.NoError(t, err)
			case <-time.After(2 * time.Second):
				require.FailNow(t, "Connect did not return in time")
			}

			output := logs.String()
			require.Contains(t, output, "daemon connected")

			if tt.wantWarning {
				assert.Contains(t, output, warning)
				assert.Contains(t, output, "node_id=1")
				assert.Contains(t, output, "version=v4.1.5")
			} else {
				assert.NotContains(t, output, warning)
			}
		})
	}
}
