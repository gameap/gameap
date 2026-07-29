package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/grpc/session"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_processMessage(t *testing.T) {
	t.Run("heartbeat_updates_last_ping_and_returns_nil", func(t *testing.T) {
		// ARRANGE
		svc, _ := newServiceWithDeps(t)
		stream := newStubStream(context.Background())
		sess := session.NewSession(1, stream, "v", nil, func() {})
		before := sess.LastPing()
		time.Sleep(2 * time.Millisecond)

		// ACT
		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_Heartbeat{Heartbeat: &proto.Heartbeat{}},
		})

		// ASSERT
		require.NoError(t, err)
		assert.True(t, sess.LastPing().After(before), "lastPing must advance on Heartbeat")
	})

	t.Run("task_status_routed_to_task_handler", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		sess := session.NewSession(7, newStubStream(context.Background()), "v", nil, func() {})
		update := &proto.TaskStatusUpdate{TaskId: 99, Status: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS}

		// ACT
		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_TaskStatus{TaskStatus: update},
		})

		// ASSERT
		require.NoError(t, err)
		got := deps.taskHandler.StatusUpdates()
		require.Len(t, got, 1)
		assert.Equal(t, uint64(99), got[0].TaskId)
	})

	t.Run("nil_task_handler_swallowed_for_task_status", func(t *testing.T) {
		// ARRANGE
		svc, _ := newServiceWithDeps(t)
		svc.taskHandler = nil
		sess := session.NewSession(7, newStubStream(context.Background()), "v", nil, func() {})

		// ACT
		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_TaskStatus{TaskStatus: &proto.TaskStatusUpdate{TaskId: 1}},
		})

		// ASSERT
		require.NoError(t, err, "nil handler must be safely ignored")
	})

	t.Run("task_output_routed_to_task_handler", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})

		// ACT
		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_TaskOutput{TaskOutput: &proto.TaskOutput{TaskId: 5, OutputChunk: []byte("o")}},
		})

		// ASSERT
		require.NoError(t, err)
		out := deps.taskHandler.Outputs()
		require.Len(t, out, 1)
		assert.Equal(t, uint64(5), out[0].TaskId)
	})

	t.Run("command_output_routed_to_command_handler", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})

		// ACT
		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_CommandOutput{
				CommandOutput: &proto.CommandOutput{CommandId: "c-1", OutputChunk: []byte("hi")},
			},
		})

		// ASSERT
		require.NoError(t, err)
		out := deps.commandHandler.Outputs()
		require.Len(t, out, 1)
		assert.Equal(t, "c-1", out[0].CommandId)
	})

	t.Run("command_result_resolves_pending_request_and_calls_handler", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})
		ch := sess.RegisterPendingRequest("req-1")

		// ACT
		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_CommandResult{
				CommandResult: &proto.CommandResult{RequestId: "req-1", ExitCode: 0},
			},
		})

		// ASSERT
		require.NoError(t, err)
		select {
		case msg := <-ch:
			require.NotNil(t, msg, "pending request must receive resolution")
			require.NotNil(t, msg.GetCommandResult())
			assert.Equal(t, int32(0), msg.GetCommandResult().ExitCode)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for pending request resolution")
		}

		results := deps.commandHandler.Results()
		require.Len(t, results, 1, "handler must still be invoked even after resolution")
	})

	t.Run("server_statuses_routed_to_handler", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})

		// ACT
		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_ServerStatuses{
				ServerStatuses: &proto.ServerStatusBatch{},
			},
		})

		// ASSERT
		require.NoError(t, err)
		batches := deps.serverHandler.Batches()
		require.Len(t, batches, 1)
	})

	t.Run("file_read_response_resolves_pending_request", func(t *testing.T) {
		// ARRANGE
		svc, _ := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})
		ch := sess.RegisterPendingRequest("file-read-1")

		// ACT
		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_FileReadResponse{
				FileReadResponse: &proto.FileReadResponse{RequestId: "file-read-1", Success: true},
			},
		})

		// ASSERT
		require.NoError(t, err)
		select {
		case msg := <-ch:
			require.NotNil(t, msg)
			require.NotNil(t, msg.GetFileReadResponse())
			assert.True(t, msg.GetFileReadResponse().Success)
		case <-time.After(time.Second):
			t.Fatal("file_read pending request not resolved")
		}
	})

	t.Run("file_write_response_resolves_pending_request", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})
		ch := sess.RegisterPendingRequest("fw-1")

		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_FileWriteResponse{
				FileWriteResponse: &proto.FileWriteResponse{RequestId: "fw-1", Success: true},
			},
		})

		require.NoError(t, err)
		select {
		case msg := <-ch:
			require.NotNil(t, msg.GetFileWriteResponse())
		case <-time.After(time.Second):
			t.Fatal("file_write not resolved")
		}
	})

	t.Run("file_list_response_resolves_pending_request", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})
		ch := sess.RegisterPendingRequest("fl-1")

		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_FileListResponse{
				FileListResponse: &proto.FileListResponse{RequestId: "fl-1"},
			},
		})

		require.NoError(t, err)
		select {
		case msg := <-ch:
			require.NotNil(t, msg.GetFileListResponse())
		case <-time.After(time.Second):
			t.Fatal("file_list not resolved")
		}
	})

	t.Run("file_operation_response_resolves_pending_request", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})
		ch := sess.RegisterPendingRequest("fo-1")

		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_FileOperationResponse{
				FileOperationResponse: &proto.FileOperationResponse{RequestId: "fo-1", Success: true},
			},
		})

		require.NoError(t, err)
		select {
		case msg := <-ch:
			require.NotNil(t, msg.GetFileOperationResponse())
		case <-time.After(time.Second):
			t.Fatal("file_operation not resolved")
		}
	})

	t.Run("status_response_resolves_pending_request", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})
		ch := sess.RegisterPendingRequest("st-1")

		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_StatusResponse{
				StatusResponse: &proto.StatusResponse{RequestId: "st-1"},
			},
		})

		require.NoError(t, err)
		select {
		case msg := <-ch:
			require.NotNil(t, msg.GetStatusResponse())
		case <-time.After(time.Second):
			t.Fatal("status not resolved")
		}
	})

	t.Run("console_log_response_resolves_pending_request", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})
		ch := sess.RegisterPendingRequest("cl-1")

		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_ConsoleLogResponse{
				ConsoleLogResponse: &proto.ConsoleLogResponse{RequestId: "cl-1"},
			},
		})

		require.NoError(t, err)
		select {
		case msg := <-ch:
			require.NotNil(t, msg.GetConsoleLogResponse())
		case <-time.After(time.Second):
			t.Fatal("console_log not resolved")
		}
	})

	t.Run("http_proxy_response_resolves_pending_request", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})
		ch := sess.RegisterPendingRequest("hp-1")

		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_HttpProxyResponse{
				HttpProxyResponse: &proto.HTTPProxyResponse{RequestId: "hp-1"},
			},
		})

		require.NoError(t, err)
		select {
		case msg := <-ch:
			require.NotNil(t, msg.GetHttpProxyResponse())
		case <-time.After(time.Second):
			t.Fatal("http_proxy not resolved")
		}
	})

	t.Run("archive_response_resolves_pending_request", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})
		ch := sess.RegisterPendingRequest("ar-1")

		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_ArchiveResponse{
				ArchiveResponse: &proto.ArchiveResponse{RequestId: "ar-1", Success: true, FilesProcessed: 3},
			},
		})

		require.NoError(t, err)
		select {
		case msg := <-ch:
			require.NotNil(t, msg.GetArchiveResponse())
			assert.True(t, msg.GetArchiveResponse().Success)
			assert.Equal(t, uint32(3), msg.GetArchiveResponse().FilesProcessed)
		case <-time.After(time.Second):
			t.Fatal("archive_response not resolved")
		}
	})

	t.Run("archive_progress_keeps_pending_request_open", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})
		ch := sess.RegisterPendingRequest("ar-2")

		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_ArchiveProgress{
				ArchiveProgress: &proto.ArchiveProgress{RequestId: "ar-2", FilesProcessed: 1},
			},
		})

		require.NoError(t, err)
		select {
		case msg, ok := <-ch:
			t.Fatalf("progress must not resolve the pending request, got msg=%v open=%v", msg, ok)
		default:
		}

		require.True(t,
			sess.ResolvePendingRequest("ar-2", &proto.DaemonMessage{
				Payload: &proto.DaemonMessage_ArchiveResponse{
					ArchiveResponse: &proto.ArchiveResponse{RequestId: "ar-2", Success: true},
				},
			}),
			"pending request must survive progress and be resolvable by the final response",
		)
		msg := <-ch
		require.NotNil(t, msg.GetArchiveResponse())
	})

	t.Run("attach_started_routed_to_attach_handler", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})

		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_AttachStarted{AttachStarted: &proto.AttachStarted{}},
		})

		require.NoError(t, err)
		deps.attachHandler.mu.Lock()
		defer deps.attachHandler.mu.Unlock()
		require.Len(t, deps.attachHandler.started, 1)
	})

	t.Run("attach_output_routed_to_attach_handler", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})

		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_AttachOutput{AttachOutput: &proto.AttachOutput{}},
		})

		require.NoError(t, err)
		deps.attachHandler.mu.Lock()
		defer deps.attachHandler.mu.Unlock()
		require.Len(t, deps.attachHandler.outputs, 1)
	})

	t.Run("attach_closed_routed_to_attach_handler", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})

		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_AttachClosed{AttachClosed: &proto.AttachClosed{}},
		})

		require.NoError(t, err)
		deps.attachHandler.mu.Lock()
		defer deps.attachHandler.mu.Unlock()
		require.Len(t, deps.attachHandler.closed, 1)
	})

	t.Run("metrics_response_routed_to_metrics_handler", func(t *testing.T) {
		svc, deps := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})

		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			RequestId: "mr-1",
			Payload:   &proto.DaemonMessage_MetricsResponse{MetricsResponse: &proto.MetricsResponse{}},
		})

		require.NoError(t, err)
		got := deps.metricsHandler.Responses()
		require.Len(t, got, 1)
	})

	t.Run("unknown_payload_returns_nil_without_panic", func(t *testing.T) {
		svc, _ := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})

		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			RequestId: "x",
			Payload:   nil,
		})

		require.NoError(t, err, "nil/unknown payload must be a no-op")
	})

	t.Run("server_task_execution_started_routed_to_server_task_handler", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		sess := session.NewSession(7, newStubStream(context.Background()), "v", nil, func() {})
		evt := &proto.ServerTaskExecutionStarted{ExecutionId: "exec-1", TaskId: 42}

		// ACT
		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_ServerTaskExecutionStarted{ServerTaskExecutionStarted: evt},
		})

		// ASSERT
		require.NoError(t, err)
		got := deps.serverTaskHandler.Started()
		require.Len(t, got, 1)
		assert.Equal(t, "exec-1", got[0].ExecutionId, "execution id must be forwarded unchanged")
		assert.Equal(t, uint64(42), got[0].TaskId)
	})

	t.Run("server_task_execution_finished_routed_to_server_task_handler", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})
		evt := &proto.ServerTaskExecutionFinished{ExecutionId: "exec-2"}

		// ACT
		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_ServerTaskExecutionFinished{ServerTaskExecutionFinished: evt},
		})

		// ASSERT
		require.NoError(t, err)
		got := deps.serverTaskHandler.Finished()
		require.Len(t, got, 1)
		assert.Equal(t, "exec-2", got[0].ExecutionId)
	})

	t.Run("server_task_execution_log_routed_to_server_task_handler", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})
		evt := &proto.ServerTaskExecutionLog{ExecutionId: "exec-3"}

		// ACT
		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_ServerTaskExecutionLog{ServerTaskExecutionLog: evt},
		})

		// ASSERT
		require.NoError(t, err)
		got := deps.serverTaskHandler.Logs()
		require.Len(t, got, 1)
		assert.Equal(t, "exec-3", got[0].ExecutionId)
	})

	t.Run("server_task_resync_request_routed_to_server_task_handler", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})
		req := &proto.ServerTaskResyncRequest{LastKnownSnapshotVersion: 17}

		// ACT
		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_ServerTaskResyncRequest{ServerTaskResyncRequest: req},
		})

		// ASSERT
		require.NoError(t, err)
		got := deps.serverTaskHandler.Resyncs()
		require.Len(t, got, 1)
		assert.Equal(t, uint64(17), got[0].LastKnownSnapshotVersion)
	})

	t.Run("nil_server_task_handler_swallowed_for_all_server_task_messages", func(t *testing.T) {
		// ARRANGE
		svc, _ := newServiceWithDeps(t)
		svc.serverTaskHandler = nil
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})

		messages := []*proto.DaemonMessage{
			{Payload: &proto.DaemonMessage_ServerTaskExecutionStarted{
				ServerTaskExecutionStarted: &proto.ServerTaskExecutionStarted{ExecutionId: "x"},
			}},
			{Payload: &proto.DaemonMessage_ServerTaskExecutionFinished{
				ServerTaskExecutionFinished: &proto.ServerTaskExecutionFinished{ExecutionId: "x"},
			}},
			{Payload: &proto.DaemonMessage_ServerTaskExecutionLog{
				ServerTaskExecutionLog: &proto.ServerTaskExecutionLog{ExecutionId: "x"},
			}},
			{Payload: &proto.DaemonMessage_ServerTaskResyncRequest{
				ServerTaskResyncRequest: &proto.ServerTaskResyncRequest{},
			}},
		}

		// ACT + ASSERT
		for _, m := range messages {
			err := svc.processMessage(context.Background(), sess, m)
			require.NoError(t, err, "nil server-task handler must be safely ignored for %T", m.Payload)
		}
	})

	t.Run("server_task_execution_started_propagates_handler_error", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		deps.serverTaskHandler.startedErr = errSentinel
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})

		// ACT
		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_ServerTaskExecutionStarted{
				ServerTaskExecutionStarted: &proto.ServerTaskExecutionStarted{ExecutionId: "exec-err"},
			},
		})

		// ASSERT
		require.Error(t, err, "handler error must be surfaced to processMessage caller")
		assert.ErrorIs(t, err, errSentinel)
	})

	t.Run("server_task_execution_finished_propagates_handler_error", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		deps.serverTaskHandler.finishedErr = errSentinel
		sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})

		// ACT
		err := svc.processMessage(context.Background(), sess, &proto.DaemonMessage{
			Payload: &proto.DaemonMessage_ServerTaskExecutionFinished{
				ServerTaskExecutionFinished: &proto.ServerTaskExecutionFinished{ExecutionId: "exec-err"},
			},
		})

		// ASSERT
		require.Error(t, err)
		assert.ErrorIs(t, err, errSentinel)
	})
}

func TestService_Connect_reconcilesAbandonedServerTaskExecutionsOnRegister(t *testing.T) {
	t.Run("forwards_in_flight_execution_ids_with_daemon_restart_reason", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		setupAuthorizedNode(t, deps, "k")

		stream := newStubConnectServer(context.Background())
		stream.PushMessage(&proto.DaemonMessage{
			Payload: &proto.DaemonMessage_Register{
				Register: &proto.RegisterRequest{
					NodeId: 1, ApiKey: "k",
					InFlightServerTaskExecutions: []*proto.InFlightServerTaskExecution{
						{ExecutionId: "exec-a"},
						{ExecutionId: "exec-b"},
					},
				},
			},
		})
		stream.CloseRecv()

		// ACT
		require.NoError(t, svc.Connect(stream))

		// ASSERT
		calls := deps.serverTaskHandler.ReconcileCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, uint64(1), calls[0].nodeID)
		assert.Equal(t, []string{"exec-a", "exec-b"}, calls[0].inFlightIDs,
			"reconcile must receive the daemon-advertised in-flight execution ids")
		assert.Equal(t, ReconcileReasonDaemonRestart, calls[0].reason)
	})

	t.Run("empty_in_flight_passes_empty_slice", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		setupAuthorizedNode(t, deps, "k")

		stream := newStubConnectServer(context.Background())
		stream.PushMessage(&proto.DaemonMessage{
			Payload: &proto.DaemonMessage_Register{
				Register: &proto.RegisterRequest{NodeId: 1, ApiKey: "k"},
			},
		})
		stream.CloseRecv()

		// ACT
		require.NoError(t, svc.Connect(stream))

		// ASSERT
		calls := deps.serverTaskHandler.ReconcileCalls()
		require.Len(t, calls, 1, "reconcile is called even when there is nothing in flight; v1 daemons never journal")
		assert.Empty(t, calls[0].inFlightIDs)
		assert.Equal(t, ReconcileReasonDaemonRestart, calls[0].reason)
	})

	t.Run("reconcile_error_does_not_fail_registration", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		setupAuthorizedNode(t, deps, "k")
		deps.serverTaskHandler.reconcileErr = errSentinel

		stream := newStubConnectServer(context.Background())
		stream.PushMessage(&proto.DaemonMessage{
			Payload: &proto.DaemonMessage_Register{
				Register: &proto.RegisterRequest{NodeId: 1, ApiKey: "k"},
			},
		})
		stream.CloseRecv()

		// ACT
		err := svc.Connect(stream)

		// ASSERT
		require.NoError(t, err, "server-task reconcile failure must not block register")
		require.Len(t, stream.Sent(), 1, "RegisterAck must still be sent")
		assert.True(t, stream.Sent()[0].GetRegisterAck().Success)
	})

	t.Run("nil_server_task_handler_skips_reconcile", func(t *testing.T) {
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		setupAuthorizedNode(t, deps, "k")
		svc.serverTaskHandler = nil

		stream := newStubConnectServer(context.Background())
		stream.PushMessage(&proto.DaemonMessage{
			Payload: &proto.DaemonMessage_Register{
				Register: &proto.RegisterRequest{
					NodeId: 1, ApiKey: "k",
					InFlightServerTaskExecutions: []*proto.InFlightServerTaskExecution{
						{ExecutionId: "ignored"},
					},
				},
			},
		})
		stream.CloseRecv()

		// ACT
		err := svc.Connect(stream)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, stream.Sent(), 1, "RegisterAck must still be sent when handler is nil")
		assert.True(t, stream.Sent()[0].GetRegisterAck().Success)
		assert.Empty(t, deps.serverTaskHandler.ReconcileCalls(),
			"the deps fake is detached when svc.serverTaskHandler is nil; no reconcile should be observable")
	})
}

func TestResolveResponse_alwaysReturnsNilAndDispatchesToSession(t *testing.T) {
	// ARRANGE
	sess := session.NewSession(1, newStubStream(context.Background()), "v", nil, func() {})
	ch := sess.RegisterPendingRequest("any-id")
	msg := &proto.DaemonMessage{RequestId: "any-id"}

	// ACT
	err := resolveResponse(sess, "any-id", msg)

	// ASSERT
	require.NoError(t, err)
	select {
	case got := <-ch:
		assert.Same(t, msg, got)
	case <-time.After(time.Second):
		t.Fatal("resolveResponse must deliver to channel")
	}
}
