package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolveOnSend waits for the next outgoing message to be observed on the
// stub stream, then resolves the matching pending session request with the
// DaemonMessage produced by `respond`. respond receives the observed
// RequestId so it can echo it back into the daemon's response.
//
// Designed to be invoked from a goroutine spawned by the test. If no
// outgoing message is observed within ~2s the function returns silently;
// the calling test will subsequently fail because its own ctx-bound
// Request* call will time out instead.
func resolveOnSend(
	deps *serviceDeps,
	stream *stubStream,
	respond func(requestID string) *proto.DaemonMessage,
) {
	const nodeID uint64 = 1
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sent := stream.Sent()
		if len(sent) > 0 && sent[0].RequestId != "" {
			requestID := sent[0].RequestId
			sess, ok := deps.registry.GetSession(nodeID)
			if !ok {
				return
			}
			msg := respond(requestID)
			sess.ResolvePendingRequest(requestID, msg)

			return
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestService_RequestFileRead(t *testing.T) {
	t.Run("not_connected_returns_error", func(t *testing.T) {
		// ARRANGE
		svc, _ := newServiceWithDeps(t)

		// ACT
		_, err := svc.RequestFileRead(context.Background(), 999, "/tmp/x", 0, 100)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node not connected")
	})

	t.Run("returns_response_when_daemon_resolves_request", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		stream := connectAndRegisterSession(t, deps)

		go resolveOnSend(deps, stream, func(reqID string) *proto.DaemonMessage {
			return &proto.DaemonMessage{
				RequestId: reqID,
				Payload: &proto.DaemonMessage_FileReadResponse{
					FileReadResponse: &proto.FileReadResponse{
						RequestId: reqID,
						Success:   true,
						Content:   []byte("hello"),
					},
				},
			}
		})

		// ACT
		resp, err := svc.RequestFileRead(context.Background(), 1, "/x", 0, 0)

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "hello", string(resp.Content))
		assert.True(t, resp.Success)

		sent := stream.Sent()
		require.Len(t, sent, 1, "single FileRead message must be queued")
		require.NotNil(t, sent[0].GetFileRead())
		assert.Equal(t, "/x", sent[0].GetFileRead().Path)
	})

	t.Run("ctx_cancel_before_response_returns_ctx_err", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		connectAndRegisterSession(t, deps)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// ACT
		_, err := svc.RequestFileRead(ctx, 1, "/x", 0, 0)

		// ASSERT
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled), "ctx.Err must propagate")
	})
}

func TestService_RequestFileWrite(t *testing.T) {
	t.Run("not_connected_returns_error", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		err := svc.RequestFileWrite(context.Background(), 1, "/x", []byte("a"), 0o644, false, daemon.OwnerOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node not connected")
	})

	t.Run("success_response_returns_nil", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		stream := connectAndRegisterSession(t, deps)

		go resolveOnSend(deps, stream, func(reqID string) *proto.DaemonMessage {
			return &proto.DaemonMessage{
				RequestId: reqID,
				Payload: &proto.DaemonMessage_FileWriteResponse{
					FileWriteResponse: &proto.FileWriteResponse{RequestId: reqID, Success: true},
				},
			}
		})

		err := svc.RequestFileWrite(context.Background(), 1, "/x", []byte("a"), 0o644, false, daemon.OwnerOptions{})
		require.NoError(t, err)
	})

	t.Run("error_response_propagates_error_text", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		stream := connectAndRegisterSession(t, deps)

		go resolveOnSend(deps, stream, func(reqID string) *proto.DaemonMessage {
			return &proto.DaemonMessage{
				RequestId: reqID,
				Payload: &proto.DaemonMessage_FileWriteResponse{
					FileWriteResponse: &proto.FileWriteResponse{RequestId: reqID, Success: false, Error: "disk full"},
				},
			}
		})

		err := svc.RequestFileWrite(context.Background(), 1, "/x", []byte("a"), 0o644, false, daemon.OwnerOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disk full")
	})
}

func TestService_RequestFileList(t *testing.T) {
	t.Run("not_connected_returns_error", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		_, err := svc.RequestFileList(context.Background(), 1, "/", false, "*")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node not connected")
	})

	t.Run("returns_response_on_resolution", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		stream := connectAndRegisterSession(t, deps)

		go resolveOnSend(deps, stream, func(reqID string) *proto.DaemonMessage {
			return &proto.DaemonMessage{
				RequestId: reqID,
				Payload: &proto.DaemonMessage_FileListResponse{
					FileListResponse: &proto.FileListResponse{RequestId: reqID, Success: true},
				},
			}
		})

		resp, err := svc.RequestFileList(context.Background(), 1, "/d", true, "*.log")
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Success)

		sent := stream.Sent()
		require.Len(t, sent, 1)
		require.NotNil(t, sent[0].GetFileList())
		assert.Equal(t, "/d", sent[0].GetFileList().Path)
		assert.True(t, sent[0].GetFileList().Recursive)
		assert.Equal(t, "*.log", sent[0].GetFileList().Pattern)
	})
}

func TestService_RequestFileOperation(t *testing.T) {
	t.Run("not_connected_returns_error", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		_, err := svc.RequestFileOperation(context.Background(), 1, &proto.FileOperationRequest{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node not connected")
	})

	t.Run("returns_response_on_resolution", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		stream := connectAndRegisterSession(t, deps)

		go resolveOnSend(deps, stream, func(reqID string) *proto.DaemonMessage {
			return &proto.DaemonMessage{
				RequestId: reqID,
				Payload: &proto.DaemonMessage_FileOperationResponse{
					FileOperationResponse: &proto.FileOperationResponse{RequestId: reqID, Success: true},
				},
			}
		})

		resp, err := svc.RequestFileOperation(context.Background(), 1, &proto.FileOperationRequest{
			Operation: proto.FileOperationType_FILE_OPERATION_TYPE_DELETE,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Success)

		sent := stream.Sent()
		require.Len(t, sent, 1)
		require.NotNil(t, sent[0].GetFileOperation())
		assert.NotEmpty(t, sent[0].GetFileOperation().RequestId,
			"RequestId must be populated on the outgoing FileOperation")
	})
}

func TestService_RequestCommand(t *testing.T) {
	t.Run("not_connected_returns_error", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		_, err := svc.RequestCommand(context.Background(), 1, &proto.CommandRequest{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node not connected")
	})

	t.Run("returns_command_result_when_resolved", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		stream := connectAndRegisterSession(t, deps)

		go resolveOnSend(deps, stream, func(reqID string) *proto.DaemonMessage {
			return &proto.DaemonMessage{
				RequestId: reqID,
				Payload: &proto.DaemonMessage_CommandResult{
					CommandResult: &proto.CommandResult{RequestId: reqID, ExitCode: 0},
				},
			}
		})

		got, err := svc.RequestCommand(context.Background(), 1, &proto.CommandRequest{Command: "ls"})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, int32(0), got.ExitCode)
	})
}

func TestService_RequestStatus(t *testing.T) {
	t.Run("not_connected_returns_error", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		_, err := svc.RequestStatus(context.Background(), 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node not connected")
	})

	t.Run("returns_status_on_resolution", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		stream := connectAndRegisterSession(t, deps)

		go resolveOnSend(deps, stream, func(reqID string) *proto.DaemonMessage {
			return &proto.DaemonMessage{
				RequestId: reqID,
				Payload: &proto.DaemonMessage_StatusResponse{
					StatusResponse: &proto.StatusResponse{RequestId: reqID, Version: "1.2.3", OnlineServers: 4},
				},
			}
		})

		got, err := svc.RequestStatus(context.Background(), 1)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "1.2.3", got.Version)
		assert.Equal(t, int32(4), got.OnlineServers)
	})
}

func TestService_RequestConsoleLog(t *testing.T) {
	t.Run("not_connected_returns_error", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		_, err := svc.RequestConsoleLog(context.Background(), 1, 5, 100)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node not connected")
	})

	t.Run("returns_log_on_resolution", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		stream := connectAndRegisterSession(t, deps)

		go resolveOnSend(deps, stream, func(reqID string) *proto.DaemonMessage {
			return &proto.DaemonMessage{
				RequestId: reqID,
				Payload: &proto.DaemonMessage_ConsoleLogResponse{
					ConsoleLogResponse: &proto.ConsoleLogResponse{RequestId: reqID, Data: []byte("output")},
				},
			}
		})

		got, err := svc.RequestConsoleLog(context.Background(), 1, 5, 100)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "output", string(got.Data))

		sent := stream.Sent()
		require.Len(t, sent, 1)
		require.NotNil(t, sent[0].GetConsoleLogRequest())
		assert.Equal(t, uint64(5), sent[0].GetConsoleLogRequest().ServerId)
		assert.Equal(t, int64(100), sent[0].GetConsoleLogRequest().MaxBytes)
	})
}

func TestService_RequestHTTPProxy(t *testing.T) {
	t.Run("not_connected_returns_error", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		_, err := svc.RequestHTTPProxy(context.Background(), 1, &proto.HTTPProxyRequest{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node not connected")
	})

	t.Run("connected_without_capability_returns_error", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		// no capability advertised
		connectAndRegisterSession(t, deps)

		_, err := svc.RequestHTTPProxy(context.Background(), 1, &proto.HTTPProxyRequest{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "http_proxy")
	})

	t.Run("returns_response_when_capability_present", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		stream := connectAndRegisterSession(t, deps, "http_proxy")

		go resolveOnSend(deps, stream, func(reqID string) *proto.DaemonMessage {
			return &proto.DaemonMessage{
				RequestId: reqID,
				Payload: &proto.DaemonMessage_HttpProxyResponse{
					HttpProxyResponse: &proto.HTTPProxyResponse{RequestId: reqID, Success: true, StatusCode: 200},
				},
			}
		})

		got, err := svc.RequestHTTPProxy(context.Background(), 1, &proto.HTTPProxyRequest{})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.True(t, got.Success)
		assert.Equal(t, int32(200), got.StatusCode)
	})
}

func TestService_RequestFileUploadTask(t *testing.T) {
	t.Run("not_connected_returns_error", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		err := svc.RequestFileUploadTask(context.Background(), 1, "tx-1", "/p", "abc", 100, 0, daemon.OwnerOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node not connected")
	})

	t.Run("success_response_returns_nil", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		stream := connectAndRegisterSession(t, deps)

		go resolveOnSend(deps, stream, func(reqID string) *proto.DaemonMessage {
			return &proto.DaemonMessage{
				RequestId: reqID,
				Payload: &proto.DaemonMessage_FileWriteResponse{
					FileWriteResponse: &proto.FileWriteResponse{RequestId: reqID, Success: true},
				},
			}
		})

		err := svc.RequestFileUploadTask(context.Background(), 1, "tx-1", "/p", "abc", 100, 0, daemon.OwnerOptions{})
		require.NoError(t, err)
	})

	t.Run("error_response_propagates_error_text", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		stream := connectAndRegisterSession(t, deps)

		go resolveOnSend(deps, stream, func(reqID string) *proto.DaemonMessage {
			return &proto.DaemonMessage{
				RequestId: reqID,
				Payload: &proto.DaemonMessage_FileWriteResponse{
					FileWriteResponse: &proto.FileWriteResponse{RequestId: reqID, Success: false, Error: "transfer failed"},
				},
			}
		})

		err := svc.RequestFileUploadTask(context.Background(), 1, "tx-1", "/p", "abc", 100, 0, daemon.OwnerOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transfer failed")
	})
}

func TestService_RequestFileDownloadTask(t *testing.T) {
	t.Run("not_connected_returns_error", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		err := svc.RequestFileDownloadTask(context.Background(), 1, "tx-1", "/src")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node not connected")
	})

	t.Run("success_response_returns_nil", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		stream := connectAndRegisterSession(t, deps)

		go resolveOnSend(deps, stream, func(reqID string) *proto.DaemonMessage {
			return &proto.DaemonMessage{
				RequestId: reqID,
				Payload: &proto.DaemonMessage_FileWriteResponse{
					FileWriteResponse: &proto.FileWriteResponse{RequestId: reqID, Success: true},
				},
			}
		})

		err := svc.RequestFileDownloadTask(context.Background(), 1, "tx-1", "/src")
		require.NoError(t, err)
	})

	t.Run("error_response_propagates_error_text", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		stream := connectAndRegisterSession(t, deps)

		go resolveOnSend(deps, stream, func(reqID string) *proto.DaemonMessage {
			return &proto.DaemonMessage{
				RequestId: reqID,
				Payload: &proto.DaemonMessage_FileWriteResponse{
					FileWriteResponse: &proto.FileWriteResponse{RequestId: reqID, Success: false, Error: "no source"},
				},
			}
		})

		err := svc.RequestFileDownloadTask(context.Background(), 1, "tx-1", "/src")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no source")
	})
}
