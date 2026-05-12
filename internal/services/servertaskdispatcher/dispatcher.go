package servertaskdispatcher

import (
	"context"
	"log/slog"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/grpc/gateway"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/idgen"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// outputInlineMaxBytes caps the inline output column the API stores on
// execution rows. Larger output is meant to flow through file-transfer
// storage; the inline column carries a truncated tail for UI preview.
const outputInlineMaxBytes = 64 * 1024

// Dispatcher owns the API-side flow of server-task scheduling. The daemon
// fires cron ticks and reports lifecycle events; this struct persists
// state and fans out realtime events to WebSocket subscribers.
type Dispatcher struct {
	sender         Sender
	serverTaskRepo repositories.ServerTaskRepository
	executionRepo  repositories.ServerTaskExecutionRepository
	publisher      pubsub.Publisher
	logger         *slog.Logger
}

func NewDispatcher(
	sender Sender,
	serverTaskRepo repositories.ServerTaskRepository,
	executionRepo repositories.ServerTaskExecutionRepository,
	publisher pubsub.Publisher,
	logger *slog.Logger,
) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}

	return &Dispatcher{
		sender:         sender,
		serverTaskRepo: serverTaskRepo,
		executionRepo:  executionRepo,
		publisher:      publisher,
		logger:         logger,
	}
}

// BuildSnapshot returns the full schedule for the given node. Called on
// daemon register (embedded in RegisterAck) and on explicit resync.
//
// Snapshot includes only enabled, non-soft-deleted tasks. Disabled tasks
// stay on the API side; daemon doesn't need them.
func (d *Dispatcher) BuildSnapshot(
	ctx context.Context, nodeID uint64,
) (*proto.ServerTaskSnapshot, error) {
	tasks, err := d.serverTaskRepo.Find(ctx, &filters.FindServerTask{
		NodeIDs:     []uint{uint(nodeID)},
		OnlyEnabled: true,
	}, nil, nil)
	if err != nil {
		return nil, errors.WithMessage(err, "load server tasks for snapshot")
	}

	return &proto.ServerTaskSnapshot{
		Tasks:       gateway.DomainServerTasksToProtoList(tasks),
		GeneratedAt: timestamppb.Now(),
		// snapshot_version: future use when we track a node-level monotonic
		// snapshot counter. v1 leaves it at 0 because per-task version
		// already handles dedup on the daemon side.
	}, nil
}

// DispatchUpsert is invoked after API persists a create/update of a task.
// It pushes a `ServerTaskDelta.upserted` over the bidi session.
//
// The caller MUST have already called serverTaskRepo.Save AND
// serverTaskRepo.BumpVersion before this so the proto carries the
// current authoritative version. We then dispatch, but a failure to
// send is logged not surfaced — the daemon will resync on reconnect.
func (d *Dispatcher) DispatchUpsert(ctx context.Context, task *domain.ServerTask) error {
	if task.NodeID == nil {
		return errors.New("server task has no node_id; cannot dispatch")
	}

	nodeID := uint64(*task.NodeID)

	msg := &proto.GatewayMessage{
		RequestId: idgen.New(),
		Payload: &proto.GatewayMessage_ServerTaskDelta{
			ServerTaskDelta: &proto.ServerTaskDelta{
				Kind: &proto.ServerTaskDelta_Upserted{
					Upserted: gateway.DomainServerTaskToProto(task),
				},
			},
		},
	}

	if err := d.sender.SendTask(ctx, nodeID, msg); err != nil {
		d.logger.Warn("failed to dispatch server task upsert; daemon will resync on reconnect",
			"task_id", task.ID, "node_id", nodeID, "error", err,
		)
	}

	return nil
}

// DispatchDelete signals the daemon to drop a task. The caller has already
// soft-deleted the task on the API side; `version` reflects the bump.
func (d *Dispatcher) DispatchDelete(
	ctx context.Context, taskID uint64, nodeID uint64, version uint64,
) error {
	msg := &proto.GatewayMessage{
		RequestId: idgen.New(),
		Payload: &proto.GatewayMessage_ServerTaskDelta{
			ServerTaskDelta: &proto.ServerTaskDelta{
				Kind: &proto.ServerTaskDelta_Deleted{
					Deleted: &proto.ServerTaskDeleted{
						Id:      taskID,
						Version: version,
					},
				},
			},
		},
	}

	if err := d.sender.SendTask(ctx, nodeID, msg); err != nil {
		d.logger.Warn("failed to dispatch server task delete; daemon will resync on reconnect",
			"task_id", taskID, "node_id", nodeID, "error", err,
		)
	}

	return nil
}

func (d *Dispatcher) HandleExecutionStarted(
	ctx context.Context, _ uint64, evt *proto.ServerTaskExecutionStarted,
) error {
	startedAt := time.Now()
	if evt.StartedAt != nil {
		startedAt = evt.StartedAt.AsTime()
	}

	exec := &domain.ServerTaskExecution{
		ExecutionID:  evt.ExecutionId,
		ServerTaskID: uint(evt.TaskId),
		ServerID:     uint(evt.ServerId),
		NodeID:       uint(evt.NodeId),
		Command:      gateway.ProtoServerTaskCommandToDomain(evt.Command),
		TaskVersion:  evt.TaskVersion,
		Status:       domain.ServerTaskExecutionStatusRunning,
		StartedAt:    startedAt,
	}

	if err := d.executionRepo.Create(ctx, exec); err != nil {
		return errors.WithMessage(err, "persist execution started")
	}

	d.publishExecutionStatus(ctx, evt.TaskId, evt.ServerId, evt.NodeId, exec.ExecutionID, exec.Status, nil, "")

	return nil
}

func (d *Dispatcher) HandleExecutionFinished(
	ctx context.Context, nodeID uint64, evt *proto.ServerTaskExecutionFinished,
) error {
	patch := repositories.ServerTaskExecutionFinishPatch{
		Status:     gateway.ProtoServerTaskExecutionStatusToDomain(evt.Status),
		FinishedAt: time.Now(),
	}
	if evt.FinishedAt != nil {
		patch.FinishedAt = evt.FinishedAt.AsTime()
	}

	if evt.ExitCode != 0 || evt.Status == proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SUCCESS {
		ec := int(evt.ExitCode)
		patch.ExitCode = &ec
	}
	if evt.ErrorMessage != "" {
		em := evt.ErrorMessage
		patch.ErrorMessage = &em
	}
	if evt.Duration != nil {
		ms := evt.Duration.AsDuration().Milliseconds()
		patch.DurationMS = &ms
	}
	if len(evt.OutputInline) > 0 {
		oi := truncateTail(string(evt.OutputInline), outputInlineMaxBytes)
		patch.OutputInline = &oi
	}
	if evt.OutputStreamed && evt.OutputStoragePath != "" {
		osp := evt.OutputStoragePath
		patch.OutputStoragePath = &osp
	}

	if err := d.executionRepo.UpdateFinish(ctx, evt.ExecutionId, patch); err != nil {
		return errors.WithMessage(err, "persist execution finish")
	}

	exitCodePtr := patch.ExitCode
	var ec32 *int32
	if exitCodePtr != nil {
		v := int32(*exitCodePtr) //nolint:gosec // exit code from daemon, fits int32
		ec32 = &v
	}

	d.publishExecutionStatus(ctx, evt.TaskId, 0, nodeID, evt.ExecutionId, patch.Status, ec32, evt.ErrorMessage)

	d.sendExecutionAck(ctx, nodeID, evt.ExecutionId)

	return nil
}

func (d *Dispatcher) HandleExecutionLog(
	ctx context.Context, nodeID uint64, evt *proto.ServerTaskExecutionLog,
) error {
	if len(evt.Chunk) == 0 && !evt.IsFinal {
		return nil
	}

	// Append a truncated tail to the inline output column. The full log
	// is meant for file-transfer storage (follow-up); for now we keep
	// the rolling tail so the executions row stays self-describing.
	if err := d.executionRepo.AppendOutputInline(ctx, evt.ExecutionId, evt.Chunk, outputInlineMaxBytes); err != nil {
		d.logger.Warn("failed to append execution output chunk",
			"execution_id", evt.ExecutionId, "node_id", nodeID, "error", err,
		)
	}

	d.publishExecutionLog(ctx, evt)

	return nil
}

func (d *Dispatcher) HandleResyncRequest(
	ctx context.Context, nodeID uint64, _ *proto.ServerTaskResyncRequest,
) error {
	snapshot, err := d.BuildSnapshot(ctx, nodeID)
	if err != nil {
		return errors.WithMessage(err, "build snapshot for resync")
	}

	msg := &proto.GatewayMessage{
		RequestId: idgen.New(),
		Payload: &proto.GatewayMessage_ServerTaskSnapshot{
			ServerTaskSnapshot: snapshot,
		},
	}

	if err := d.sender.SendTask(ctx, nodeID, msg); err != nil {
		return errors.WithMessage(err, "send snapshot after resync request")
	}

	return nil
}

func (d *Dispatcher) ReconcileWorkingExecutions(
	ctx context.Context, nodeID uint64, inFlightExecIDs []string, reason string,
) (int, error) {
	if reason == "" {
		reason = domain.ExecutionAbandonedReasonDaemonRestart
	}

	n, err := d.executionRepo.MarkAbandoned(ctx, uint(nodeID), inFlightExecIDs, reason)
	if err != nil {
		return 0, errors.WithMessage(err, "mark abandoned executions")
	}

	if n > 0 {
		d.logger.Info("reconciled abandoned server task executions",
			"node_id", nodeID, "count", n, "reason", reason,
		)
	}

	return n, nil
}

// CancelExecution asks the daemon to abort an in-flight execution.
// Persisting the canceled state happens when the daemon reports back via
// `ServerTaskExecutionFinished` with status=CANCELED; this method only
// sends the request.
func (d *Dispatcher) CancelExecution(
	ctx context.Context, executionID string, taskID uint64, nodeID uint64, reason string,
) error {
	msg := &proto.GatewayMessage{
		RequestId: idgen.New(),
		Payload: &proto.GatewayMessage_ServerTaskExecutionCancel{
			ServerTaskExecutionCancel: &proto.ServerTaskExecutionCancel{
				ExecutionId: executionID,
				TaskId:      taskID,
				Reason:      reason,
			},
		},
	}

	if err := d.sender.SendTask(ctx, nodeID, msg); err != nil {
		return errors.WithMessage(err, "send execution cancel")
	}

	return nil
}

func (d *Dispatcher) sendExecutionAck(ctx context.Context, nodeID uint64, executionID string) {
	msg := &proto.GatewayMessage{
		RequestId: idgen.New(),
		Payload: &proto.GatewayMessage_ServerTaskExecutionAck{
			ServerTaskExecutionAck: &proto.ServerTaskExecutionAck{
				ExecutionId: executionID,
			},
		},
	}

	if err := d.sender.SendTask(ctx, nodeID, msg); err != nil {
		d.logger.Warn("failed to send execution ack",
			"execution_id", executionID, "node_id", nodeID, "error", err,
		)
	}
}

func (d *Dispatcher) publishExecutionStatus(
	ctx context.Context,
	taskID, serverID, nodeID uint64,
	executionID string,
	status domain.ServerTaskExecutionStatus,
	exitCode *int32,
	errorMessage string,
) {
	if d.publisher == nil {
		return
	}

	channel := channels.BuildRealtimeServerTaskExecutionChannel(taskID)
	payload := messages.ServerTaskExecutionStatusPayload{
		ExecutionID:  executionID,
		TaskID:       taskID,
		ServerID:     serverID,
		NodeID:       nodeID,
		Status:       string(status),
		ExitCode:     exitCode,
		ErrorMessage: errorMessage,
	}

	msgType := messages.TypeServerTaskExecutionStarted
	if status != domain.ServerTaskExecutionStatusRunning {
		msgType = messages.TypeServerTaskExecutionFinished
	}

	msg, err := messages.NewMessage(channel, msgType, payload)
	if err != nil {
		d.logger.Warn("failed to build execution status message", "error", err)

		return
	}

	if err := d.publisher.Publish(ctx, channel, msg); err != nil {
		d.logger.Warn("failed to publish execution status",
			"task_id", taskID, "execution_id", executionID, "error", err,
		)
	}
}

func (d *Dispatcher) publishExecutionLog(ctx context.Context, evt *proto.ServerTaskExecutionLog) {
	if d.publisher == nil {
		return
	}

	channel := channels.BuildRealtimeServerTaskLogChannel(evt.ExecutionId)
	payload := messages.ServerTaskExecutionLogPayload{
		ExecutionID: evt.ExecutionId,
		Sequence:    evt.Sequence,
		Chunk:       evt.Chunk,
		IsFinal:     evt.IsFinal,
	}

	msg, err := messages.NewMessage(channel, messages.TypeServerTaskExecutionLog, payload)
	if err != nil {
		d.logger.Warn("failed to build execution log message", "error", err)

		return
	}

	if err := d.publisher.Publish(ctx, channel, msg); err != nil {
		d.logger.Warn("failed to publish execution log",
			"execution_id", evt.ExecutionId, "error", err,
		)
	}
}

func truncateTail(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}

	return s[len(s)-maxBytes:]
}
