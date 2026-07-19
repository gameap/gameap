package taskdispatcher

import (
	"context"
	"log/slog"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/grpc/gateway"
	"github.com/gameap/gameap/internal/grpc/session"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/idgen"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
)

type Dispatcher struct {
	registry          *session.Registry
	daemonTaskRepo    repositories.DaemonTaskRepository
	serverRepo        repositories.ServerRepository
	serverSettingRepo repositories.ServerSettingRepository
	gameRepo          repositories.GameRepository
	gameModRepo       repositories.GameModRepository
	nodeRepo          repositories.NodeRepository
	publisher         pubsub.Publisher
	pluginEvents      PluginEventDispatcher
	logger            *slog.Logger
}

func NewDispatcher(
	registry *session.Registry,
	daemonTaskRepo repositories.DaemonTaskRepository,
	serverRepo repositories.ServerRepository,
	serverSettingRepo repositories.ServerSettingRepository,
	gameRepo repositories.GameRepository,
	gameModRepo repositories.GameModRepository,
	nodeRepo repositories.NodeRepository,
	publisher pubsub.Publisher,
	logger *slog.Logger,
) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}

	return &Dispatcher{
		registry:          registry,
		daemonTaskRepo:    daemonTaskRepo,
		serverRepo:        serverRepo,
		serverSettingRepo: serverSettingRepo,
		gameRepo:          gameRepo,
		gameModRepo:       gameModRepo,
		nodeRepo:          nodeRepo,
		publisher:         publisher,
		logger:            logger,
	}
}

// SetPluginEventDispatcher wires plugin task events. It is a setter (not a
// constructor argument) to break the container cycle: the plugin manager's
// host libraries depend on this dispatcher.
func (d *Dispatcher) SetPluginEventDispatcher(pluginEvents PluginEventDispatcher) {
	d.pluginEvents = pluginEvents
}

func (d *Dispatcher) Dispatch(ctx context.Context, task *domain.DaemonTask) error {
	if err := d.daemonTaskRepo.Save(ctx, task); err != nil {
		return errors.Wrap(err, "persist task")
	}

	d.dispatchTaskCreatedEvent(ctx, task)

	d.sendServerConfigUpdate(ctx, task)

	protoTask := gateway.DomainDaemonTaskToProto(task)
	msg := &proto.GatewayMessage{
		RequestId: idgen.New(),
		Payload: &proto.GatewayMessage_Task{
			Task: protoTask,
		},
	}

	err := d.registry.SendTask(ctx, uint64(task.DedicatedServerID), msg)
	if err == nil {
		if err := d.daemonTaskRepo.Save(ctx, task); err != nil {
			d.logger.Warn("failed to update task status after dispatch",
				"task_id", task.ID,
				"error", err,
			)
		}
		d.logger.Info("task dispatched",
			"task_id", task.ID,
			"node_id", task.DedicatedServerID,
		)

		return nil
	}

	d.logger.Warn("task dispatch failed, will retry on reconnect",
		"task_id", task.ID,
		"node_id", task.DedicatedServerID,
		"error", err,
	)

	return nil
}

func (d *Dispatcher) FlushPending(ctx context.Context, nodeID uint64) error {
	sess, ok := d.registry.GetSession(nodeID)
	if !ok {
		return errors.New("session not found")
	}

	tasks, err := d.daemonTaskRepo.Find(ctx, &filters.FindDaemonTask{
		DedicatedServerIDs: []uint{uint(nodeID)},
		Statuses:           []domain.DaemonTaskStatus{domain.DaemonTaskStatusWaiting},
	}, nil, nil)
	if err != nil {
		return errors.Wrap(err, "find pending tasks")
	}

	for i := range tasks {
		task := &tasks[i]

		d.sendServerConfigUpdate(ctx, task)

		protoTask := gateway.DomainDaemonTaskToProto(task)
		msg := &proto.GatewayMessage{
			RequestId: idgen.New(),
			Payload: &proto.GatewayMessage_Task{
				Task: protoTask,
			},
		}

		if err := sess.Send(msg); err != nil {
			d.logger.Error("failed to send pending task",
				"task_id", task.ID,
				"node_id", nodeID,
				"error", err,
			)

			return errors.Wrap(err, "send pending task")
		}

		task.Status = domain.DaemonTaskStatusWorking
		if err := d.daemonTaskRepo.Save(ctx, task); err != nil {
			d.logger.Warn("failed to update task status",
				"task_id", task.ID,
				"error", err,
			)
		}
	}

	d.logger.Info("flushed pending tasks",
		"node_id", nodeID,
		"count", len(tasks),
	)

	return nil
}

func (d *Dispatcher) GetPendingTasks(ctx context.Context, nodeID uint64) ([]*proto.DaemonTask, error) {
	tasks, err := d.daemonTaskRepo.Find(ctx, &filters.FindDaemonTask{
		DedicatedServerIDs: []uint{uint(nodeID)},
		Statuses:           []domain.DaemonTaskStatus{domain.DaemonTaskStatusWaiting},
	}, nil, nil)
	if err != nil {
		return nil, errors.Wrap(err, "find pending tasks")
	}

	protoTasks := make([]*proto.DaemonTask, 0, len(tasks))
	for i := range tasks {
		protoTasks = append(protoTasks, gateway.DomainDaemonTaskToProto(&tasks[i]))
	}

	return protoTasks, nil
}

func (d *Dispatcher) HandleTaskStatusUpdate(ctx context.Context, nodeID uint64, update *proto.TaskStatusUpdate) error {
	tasks, err := d.daemonTaskRepo.Find(ctx, &filters.FindDaemonTask{
		IDs: []uint{uint(update.TaskId)},
	}, nil, nil)
	if err != nil {
		return errors.Wrap(err, "find task")
	}

	if len(tasks) == 0 {
		d.logger.Warn("task not found for status update",
			"task_id", update.TaskId,
			"node_id", nodeID,
		)

		return nil
	}

	task := &tasks[0]

	if uint(nodeID) != task.DedicatedServerID {
		d.logger.Warn("task status update from wrong node",
			"task_id", update.TaskId,
			"expected_node_id", task.DedicatedServerID,
			"actual_node_id", nodeID,
		)

		return nil
	}

	task.Status = gateway.ProtoTaskStatusToDomain(update.Status)
	if err := d.daemonTaskRepo.Save(ctx, task); err != nil {
		return errors.Wrap(err, "update task status")
	}

	d.publishTaskStatus(ctx, update.TaskId, string(task.Status), task.DedicatedServerID, update.Message)

	d.logger.Debug("task status updated",
		"task_id", task.ID,
		"status", task.Status,
	)

	return nil
}

func (d *Dispatcher) HandleTaskOutput(ctx context.Context, _ uint64, output *proto.TaskOutput) error {
	if len(output.OutputChunk) == 0 {
		return nil
	}

	if err := d.daemonTaskRepo.AppendOutput(ctx, uint(output.TaskId), string(output.OutputChunk)); err != nil {
		return errors.Wrap(err, "append task output")
	}

	d.publishTaskOutput(ctx, output.TaskId, string(output.OutputChunk), output.IsFinal)

	return nil
}

func (d *Dispatcher) CancelTask(ctx context.Context, taskID uint64, reason string) error {
	tasks, err := d.daemonTaskRepo.Find(ctx, &filters.FindDaemonTask{
		IDs: []uint{uint(taskID)},
	}, nil, nil)
	if err != nil {
		return errors.Wrap(err, "find task")
	}

	if len(tasks) == 0 {
		return errors.New("task not found")
	}

	task := &tasks[0]
	nodeID := uint64(task.DedicatedServerID)

	msg := &proto.GatewayMessage{
		RequestId: idgen.New(),
		Payload: &proto.GatewayMessage_TaskCancel{
			TaskCancel: &proto.TaskCancel{
				TaskId: taskID,
				Reason: reason,
			},
		},
	}

	if err := d.registry.SendTask(ctx, nodeID, msg); err != nil {
		d.logger.Warn("failed to send task cancel",
			"task_id", taskID,
			"node_id", nodeID,
			"error", err,
		)
	}

	task.Status = domain.DaemonTaskStatusCanceled
	if err := d.daemonTaskRepo.Save(ctx, task); err != nil {
		return errors.Wrap(err, "update task status")
	}

	return nil
}

func (d *Dispatcher) dispatchTaskCreatedEvent(ctx context.Context, task *domain.DaemonTask) {
	if d.pluginEvents == nil {
		return
	}

	d.pluginEvents.DispatchTaskEventAsync(
		ctx,
		pluginproto.EventType_EVENT_TYPE_DAEMON_TASK_CREATED,
		task.ID,
		task.DedicatedServerID,
		task.ServerID,
		string(task.Task),
		string(task.Status),
		nil,
	)
}

func (d *Dispatcher) publishTaskStatus(
	ctx context.Context, taskID uint64, status string, serverID uint, message string,
) {
	if d.publisher == nil {
		return
	}

	channel := channels.BuildRealtimeTaskStatusChannel(taskID)

	msg, err := messages.NewMessage(channel, messages.TypeTaskStatus, messages.TaskStatusPayload{
		TaskID:   taskID,
		Status:   status,
		ServerID: serverID,
		Message:  message,
	})
	if err != nil {
		d.logger.Warn("failed to create task status message", "error", err)

		return
	}

	if err := d.publisher.Publish(ctx, channel, msg); err != nil {
		d.logger.Warn("failed to publish task status", "task_id", taskID, "error", err)
	}
}

func (d *Dispatcher) publishTaskOutput(ctx context.Context, taskID uint64, chunk string, isFinal bool) {
	if d.publisher == nil {
		return
	}

	channel := channels.BuildRealtimeTaskOutputChannel(taskID)

	msg, err := messages.NewMessage(channel, messages.TypeTaskOutput, messages.TaskOutputPayload{
		TaskID:  taskID,
		Chunk:   chunk,
		IsFinal: isFinal,
	})
	if err != nil {
		d.logger.Warn("failed to create task output message", "error", err)

		return
	}

	if err := d.publisher.Publish(ctx, channel, msg); err != nil {
		d.logger.Warn("failed to publish task output", "task_id", taskID, "error", err)
	}
}

func (d *Dispatcher) sendServerConfigUpdate(ctx context.Context, task *domain.DaemonTask) {
	if task.ServerID == nil {
		return
	}

	servers, err := d.serverRepo.Find(ctx, &filters.FindServer{
		IDs: []uint{*task.ServerID},
	}, nil, nil)
	if err != nil || len(servers) == 0 {
		d.logger.Warn("failed to load server for config update",
			"server_id", *task.ServerID,
			"error", err,
		)

		return
	}

	server := &servers[0]

	var gameMod *domain.GameMod
	gameMods, gmErr := d.gameModRepo.Find(ctx, &filters.FindGameMod{
		IDs: []uint{server.GameModID},
	}, nil, nil)
	if gmErr != nil {
		d.logger.Warn("failed to load game mod for config update",
			"game_mod_id", server.GameModID,
			"error", gmErr,
		)
	} else if len(gameMods) > 0 {
		gameMod = &gameMods[0]
	}

	var nodeOS domain.NodeOS
	nodes, nodeErr := d.nodeRepo.Find(ctx, &filters.FindNode{
		IDs: []uint{server.DSID},
	}, nil, nil)
	if nodeErr != nil {
		d.logger.Warn("failed to load node for config update",
			"node_id", server.DSID,
			"error", nodeErr,
		)
	} else if len(nodes) > 0 {
		nodeOS = nodes[0].OS
	}

	settings, err := d.serverSettingRepo.Find(ctx, &filters.FindServerSetting{
		ServerIDs: []uint{*task.ServerID},
	}, nil, nil)
	if err != nil {
		d.logger.Warn("failed to load server settings for config update",
			"server_id", *task.ServerID,
			"error", err,
		)
	}

	var game *domain.Game
	if d.gameRepo != nil {
		games, gErr := d.gameRepo.Find(ctx, filters.FindGameByCodes(server.GameID), nil, nil)
		if gErr != nil {
			d.logger.Warn("failed to load game for config update",
				"game_id", server.GameID,
				"error", gErr,
			)
		} else if len(games) > 0 {
			game = &games[0]
		}
	}

	update := &proto.ServerConfigUpdate{
		Server:   gateway.DomainServerToProtoWithGameMod(server, gameMod, nodeOS),
		Settings: gateway.DomainServerSettingsToProto(settings),
	}

	if game != nil {
		update.Game = gateway.DomainGameToProto(game)
	}
	if gameMod != nil {
		update.GameMod = gateway.DomainGameModToProto(gameMod)
	}

	configMsg := &proto.GatewayMessage{
		RequestId: idgen.New(),
		Payload: &proto.GatewayMessage_ServerConfigUpdate{
			ServerConfigUpdate: update,
		},
	}

	if err := d.registry.SendTask(ctx, uint64(task.DedicatedServerID), configMsg); err != nil {
		d.logger.Warn("failed to send server config update",
			"server_id", *task.ServerID,
			"node_id", task.DedicatedServerID,
			"error", err,
		)
	}
}
