package hostlibrary

import (
	"context"
	"log/slog"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/daemontasks"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/samber/lo"
	"github.com/tetratelabs/wazero"
)

var domainToProtoStatus = map[domain.DaemonTaskStatus]proto.DaemonTaskStatus{
	domain.DaemonTaskStatusWaiting:  proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WAITING,
	domain.DaemonTaskStatusWorking:  proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING,
	domain.DaemonTaskStatusError:    proto.DaemonTaskStatus_DAEMON_TASK_STATUS_ERROR,
	domain.DaemonTaskStatusSuccess:  proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS,
	domain.DaemonTaskStatusCanceled: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_CANCELED,
}

var protoToDomainStatus = map[proto.DaemonTaskStatus]domain.DaemonTaskStatus{
	proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WAITING:  domain.DaemonTaskStatusWaiting,
	proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING:  domain.DaemonTaskStatusWorking,
	proto.DaemonTaskStatus_DAEMON_TASK_STATUS_ERROR:    domain.DaemonTaskStatusError,
	proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS:  domain.DaemonTaskStatusSuccess,
	proto.DaemonTaskStatus_DAEMON_TASK_STATUS_CANCELED: domain.DaemonTaskStatusCanceled,
}

var domainToProtoType = map[domain.DaemonTaskType]proto.DaemonTaskType{
	domain.DaemonTaskTypeServerStart:   proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_START,
	domain.DaemonTaskTypeServerStop:    proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_STOP,
	domain.DaemonTaskTypeServerRestart: proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_RESTART,
	domain.DaemonTaskTypeServerUpdate:  proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_UPDATE,
	domain.DaemonTaskTypeServerInstall: proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_INSTALL,
	domain.DaemonTaskTypeServerDelete:  proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_DELETE,
	domain.DaemonTaskTypeServerMove:    proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_MOVE,
	domain.DaemonTaskTypeCmdExec:       proto.DaemonTaskType_DAEMON_TASK_TYPE_CMD_EXEC,
}

var protoToDomainType = map[proto.DaemonTaskType]domain.DaemonTaskType{
	proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_START:   domain.DaemonTaskTypeServerStart,
	proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_STOP:    domain.DaemonTaskTypeServerStop,
	proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_RESTART: domain.DaemonTaskTypeServerRestart,
	proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_UPDATE:  domain.DaemonTaskTypeServerUpdate,
	proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_INSTALL: domain.DaemonTaskTypeServerInstall,
	proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_DELETE:  domain.DaemonTaskTypeServerDelete,
	proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_MOVE:    domain.DaemonTaskTypeServerMove,
	proto.DaemonTaskType_DAEMON_TASK_TYPE_CMD_EXEC:       domain.DaemonTaskTypeCmdExec,
}

type TaskDispatcher interface {
	Dispatch(ctx context.Context, task *domain.DaemonTask) error
}

// DaemonTasksServiceImpl is per plugin: reading tasks is open, creating one
// is gated on manage_servers (and node_commands for cmdexec, which runs an
// arbitrary command on the node), rate limited and audited.
type DaemonTasksServiceImpl struct {
	daemonTaskRepo repositories.DaemonTaskRepository
	taskDispatcher TaskDispatcher
	guard          *PluginGuard
}

func NewDaemonTasksService(
	daemonTaskRepo repositories.DaemonTaskRepository,
	taskDispatcher TaskDispatcher,
	guard *PluginGuard,
) *DaemonTasksServiceImpl {
	return &DaemonTasksServiceImpl{
		daemonTaskRepo: daemonTaskRepo,
		taskDispatcher: taskDispatcher,
		guard:          guard,
	}
}

func (s *DaemonTasksServiceImpl) FindDaemonTasks(
	ctx context.Context,
	req *daemontasks.FindDaemonTasksRequest,
) (*daemontasks.FindDaemonTasksResponse, error) {
	var filter *filters.FindDaemonTask
	if req.Filter != nil {
		filter = &filters.FindDaemonTask{
			IDs:                uintsFromUint64s(req.Filter.Ids),
			DedicatedServerIDs: uintsFromUint64s(req.Filter.NodeIds),
			ServerIDs:          uintPtrsFromUint64s(req.Filter.ServerIds),
			Statuses:           convertProtoStatusesToDomain(req.Filter.Statuses),
			Tasks:              convertProtoTypesToDomain(req.Filter.TaskTypes),
		}
	}

	var pagination *filters.Pagination
	if req.Pagination != nil {
		pagination = &filters.Pagination{
			Limit:  uint64(req.Pagination.Limit),
			Offset: uint64(req.Pagination.Offset),
		}
	}

	sorting := convertSorting(req.Sorting)

	tasks, err := s.daemonTaskRepo.Find(ctx, filter, sorting, pagination)
	if err != nil {
		return nil, err
	}

	return &daemontasks.FindDaemonTasksResponse{
		Tasks: convertDaemonTasksToProto(tasks),
		Total: int32(len(tasks)), //nolint:gosec
	}, nil
}

func (s *DaemonTasksServiceImpl) CreateDaemonTask(
	ctx context.Context,
	req *daemontasks.CreateDaemonTaskRequest,
) (*daemontasks.CreateDaemonTaskResponse, error) {
	if msg := s.guard.Check(ctx, ModuleDaemonTasks, "create_daemon_task"); msg != "" {
		return &daemontasks.CreateDaemonTaskResponse{Success: false, Error: new(msg)}, nil
	}

	taskType, ok := protoToDomainType[req.TaskType]
	if !ok {
		return &daemontasks.CreateDaemonTaskResponse{
			Success: false,
			Error:   new("invalid task type"),
		}, nil
	}

	// A cmdexec task is a shell command on the node: the same capability as
	// gameap-nodecmd, gated by the same grant.
	if taskType == domain.DaemonTaskTypeCmdExec {
		if msg := s.guard.Require(ctx, ModuleDaemonTasks, "create_daemon_task",
			domain.PluginPermissionNodeCommands); msg != "" {
			return &daemontasks.CreateDaemonTaskResponse{Success: false, Error: new(msg)}, nil
		}
	}

	task := &domain.DaemonTask{
		DedicatedServerID: uint(req.NodeId),
		Task:              taskType,
		Status:            domain.DaemonTaskStatusWaiting,
	}

	if req.ServerId != nil {
		serverID := uint(*req.ServerId)
		task.ServerID = &serverID
	}

	if req.RunAfterId != nil {
		runAfterID := uint(*req.RunAfterId)
		task.RunAftID = &runAfterID
	}

	if req.Cmd != nil {
		task.Cmd = req.Cmd
	}

	var err error
	if s.taskDispatcher != nil {
		err = s.taskDispatcher.Dispatch(ctx, task)
	} else {
		err = s.daemonTaskRepo.Save(ctx, task)
	}

	attrs := []slog.Attr{slog.String("task_type", string(taskType))}
	if task.ServerID != nil {
		attrs = append(attrs, slog.Uint64("server_id", uint64(*task.ServerID)))
	}
	if err == nil {
		attrs = append(attrs, slog.Uint64("task_id", uint64(task.ID)))
	}

	s.guard.Audit(ctx, audit.EventPluginTaskCreate, "create", "node", nodeResourceID(req.NodeId), err, attrs...)

	if err != nil {
		return &daemontasks.CreateDaemonTaskResponse{
			Success: false,
			Error:   new(err.Error()),
		}, nil
	}

	return &daemontasks.CreateDaemonTaskResponse{
		Success: true,
		TaskId:  uint64(task.ID),
	}, nil
}

func convertProtoStatusesToDomain(statuses []proto.DaemonTaskStatus) []domain.DaemonTaskStatus {
	return lo.FilterMap(statuses, func(s proto.DaemonTaskStatus, _ int) (domain.DaemonTaskStatus, bool) {
		ds, ok := protoToDomainStatus[s]

		return ds, ok
	})
}

func convertProtoTypesToDomain(types []proto.DaemonTaskType) []domain.DaemonTaskType {
	return lo.FilterMap(types, func(t proto.DaemonTaskType, _ int) (domain.DaemonTaskType, bool) {
		dt, ok := protoToDomainType[t]

		return dt, ok
	})
}

func convertDaemonTasksToProto(tasks []domain.DaemonTask) []*proto.DaemonTask {
	return lo.Map(tasks, func(t domain.DaemonTask, _ int) *proto.DaemonTask {
		return convertDaemonTaskToProto(&t)
	})
}

func convertDaemonTaskToProto(t *domain.DaemonTask) *proto.DaemonTask {
	var serverID, runAfterID *uint64
	if t.ServerID != nil {
		serverID = new(uint64(*t.ServerID))
	}
	if t.RunAftID != nil {
		runAfterID = new(uint64(*t.RunAftID))
	}

	return &proto.DaemonTask{
		Id:         uint64(t.ID),
		NodeId:     uint64(t.DedicatedServerID),
		ServerId:   serverID,
		RunAfterId: runAfterID,
		TaskType:   domainToProtoType[t.Task],
		Cmd:        t.Cmd,
		Output:     t.Output,
		Status:     domainToProtoStatus[t.Status],
	}
}

type DaemonTasksHostLibrary struct {
	impl *DaemonTasksServiceImpl
}

func (l *DaemonTasksHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return daemontasks.Instantiate(ctx, r, l.impl)
}

// DaemonTasksHostLibraryFactory builds a per-plugin daemontasks module bound
// to the plugin's guard.
type DaemonTasksHostLibraryFactory struct {
	daemonTaskRepo repositories.DaemonTaskRepository
	taskDispatcher TaskDispatcher
	guard          *Guard
}

func NewDaemonTasksHostLibraryFactory(
	daemonTaskRepo repositories.DaemonTaskRepository,
	taskDispatcher TaskDispatcher,
	guard *Guard,
) *DaemonTasksHostLibraryFactory {
	return &DaemonTasksHostLibraryFactory{
		daemonTaskRepo: daemonTaskRepo,
		taskDispatcher: taskDispatcher,
		guard:          guard,
	}
}

func (f *DaemonTasksHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return &DaemonTasksHostLibrary{
		impl: NewDaemonTasksService(f.daemonTaskRepo, f.taskDispatcher, f.guard.For(pluginID)),
	}
}
