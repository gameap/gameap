package channels

import (
	"strconv"
)

const (
	Prefix = "gameap:"

	CachePrefix          = Prefix + "cache:"
	CacheInvalidate      = CachePrefix + "invalidate"
	CacheInvalidateGames = CachePrefix + "invalidate:games"
	CacheInvalidateUsers = CachePrefix + "invalidate:users"
	CacheInvalidateNodes = CachePrefix + "invalidate:nodes"
	CacheInvalidateRBAC  = CachePrefix + "invalidate:rbac"
	CacheInvalidateAll   = CachePrefix + "invalidate:*"

	PluginPrefix       = Prefix + "plugin:"
	PluginEvents       = PluginPrefix + "events"
	PluginServerEvents = PluginPrefix + "events:server"
	PluginTaskEvents   = PluginPrefix + "events:task"
	// PluginSync nudges every instance to reconcile its loaded plugins against
	// the database after an install, update, uninstall or status change.
	PluginSync = PluginPrefix + "sync"

	RealtimePrefix        = Prefix + "realtime:"
	RealtimeServerStatus  = RealtimePrefix + "server:status"
	RealtimeTaskProgress  = RealtimePrefix + "task:progress"
	RealtimeNotifications = RealtimePrefix + "notifications"

	RealtimeTaskStatus    = RealtimePrefix + "task:status:"
	RealtimeTaskOutput    = RealtimePrefix + "task:output:"
	RealtimeTaskAll       = RealtimePrefix + "task:*"
	RealtimeConsoleOutput = RealtimePrefix + "console:output:"
	RealtimeConsoleResult = RealtimePrefix + "console:result:"
	RealtimeConsoleAll    = RealtimePrefix + "console:*"

	SystemPrefix       = Prefix + "system:"
	SystemShutdown     = SystemPrefix + "shutdown"
	SystemConfigReload = SystemPrefix + "config:reload"

	DaemonPrefix           = Prefix + "daemon:"
	DaemonSessionConnected = DaemonPrefix + "session:connected"
	DaemonSessionClosed    = DaemonPrefix + "session:closed"
	DaemonTaskDispatch     = DaemonPrefix + "task:dispatch:"
	DaemonCommandDispatch  = DaemonPrefix + "command:dispatch:"
	DaemonSessionAll       = DaemonPrefix + "session:*"
	DaemonTaskDispatchAll  = DaemonPrefix + "task:dispatch:*"

	DaemonFileRequest          = DaemonPrefix + "file:request:"
	DaemonFileResponse         = DaemonPrefix + "file:response:"
	DaemonFileRequestAll       = DaemonPrefix + "file:request:*"
	DaemonFileTransferComplete = DaemonPrefix + "file:transfer:complete:"

	DaemonCommandRequest    = DaemonPrefix + "command:request:"
	DaemonCommandResponse   = DaemonPrefix + "command:response:"
	DaemonCommandRequestAll = DaemonPrefix + "command:request:*"

	RealtimeAttachStarted   = RealtimePrefix + "attach:started:"
	RealtimeAttachOutput    = RealtimePrefix + "attach:output:"
	RealtimeAttachClosed    = RealtimePrefix + "attach:closed:"
	RealtimeAttachAll       = RealtimePrefix + "attach:*"
	DaemonAttachDispatch    = DaemonPrefix + "attach:dispatch:"
	DaemonAttachDispatchAll = DaemonPrefix + "attach:dispatch:*"

	DaemonConsoleLogRequest    = DaemonPrefix + "consolelog:request:"
	DaemonConsoleLogResponse   = DaemonPrefix + "consolelog:response:"
	DaemonConsoleLogRequestAll = DaemonPrefix + "consolelog:request:*"

	DaemonStatusRequest    = DaemonPrefix + "status:request:"
	DaemonStatusResponse   = DaemonPrefix + "status:response:"
	DaemonStatusRequestAll = DaemonPrefix + "status:request:*"

	DaemonHTTPProxyRequest    = DaemonPrefix + "httpproxy:request:"
	DaemonHTTPProxyResponse   = DaemonPrefix + "httpproxy:response:"
	DaemonHTTPProxyRequestAll = DaemonPrefix + "httpproxy:request:*"

	RealtimeMetrics    = RealtimePrefix + "metrics:"
	RealtimeMetricsAll = RealtimePrefix + "metrics:*"

	DaemonMetricsRequest    = DaemonPrefix + "metrics:request:"
	DaemonMetricsResponse   = DaemonPrefix + "metrics:response:"
	DaemonMetricsRequestAll = DaemonPrefix + "metrics:request:*"

	MetricsSubscribers    = Prefix + "metrics:subscribers:"
	MetricsSubscribersAll = Prefix + "metrics:subscribers:*"

	DaemonServerTaskDelta     = DaemonPrefix + "server_task:delta:"
	DaemonServerTaskDeltaAll  = DaemonPrefix + "server_task:delta:*"
	DaemonServerTaskResync    = DaemonPrefix + "server_task:resync:"
	DaemonServerTaskResyncAll = DaemonPrefix + "server_task:resync:*"

	RealtimeServerTaskExecution = RealtimePrefix + "server_task:execution:"
	RealtimeServerTaskLog       = RealtimePrefix + "server_task:log:"
	RealtimeServerTaskAll       = RealtimePrefix + "server_task:*"

	// RealtimeArchiveOp carries progress/final events of one daemon archive
	// operation; consumed by the initiating instance (registry, plugin
	// callbacks). Not bridged to WebSocket.
	RealtimeArchiveOp    = RealtimePrefix + "archive:op:"
	RealtimeArchiveOpAll = RealtimePrefix + "archive:op:*"
	// RealtimeFMArchive duplicates archive events of server-scoped (file
	// manager) operations, keyed by server ID; bridged to WebSocket.
	RealtimeFMArchive    = RealtimePrefix + "fm:archive:"
	RealtimeFMArchiveAll = RealtimePrefix + "fm:archive:*"

	DaemonArchiveRequest    = DaemonPrefix + "archive:request:"
	DaemonArchiveResponse   = DaemonPrefix + "archive:response:"
	DaemonArchiveRequestAll = DaemonPrefix + "archive:request:*"
)

func BuildCacheInvalidateChannel(entityType string, entityID string) string {
	if entityID == "" {
		return CachePrefix + "invalidate:" + entityType
	}

	return CachePrefix + "invalidate:" + entityType + ":" + entityID
}

func BuildPluginEventChannel(eventType string) string {
	return PluginPrefix + "events:" + eventType
}

func BuildDaemonTaskDispatchChannel(nodeID uint64) string {
	return DaemonTaskDispatch + strconv.FormatUint(nodeID, 10)
}

func BuildDaemonCommandDispatchChannel(nodeID uint64) string {
	return DaemonCommandDispatch + strconv.FormatUint(nodeID, 10)
}

func BuildRealtimeTaskStatusChannel(taskID uint64) string {
	return RealtimeTaskStatus + strconv.FormatUint(taskID, 10)
}

func BuildRealtimeTaskOutputChannel(taskID uint64) string {
	return RealtimeTaskOutput + strconv.FormatUint(taskID, 10)
}

func BuildRealtimeConsoleOutputChannel(serverID uint64) string {
	return RealtimeConsoleOutput + strconv.FormatUint(serverID, 10)
}

func BuildDaemonFileRequestChannel(nodeID uint64) string {
	return DaemonFileRequest + strconv.FormatUint(nodeID, 10)
}

func BuildDaemonFileResponseChannel(instanceID string) string {
	return DaemonFileResponse + instanceID
}

func BuildDaemonFileTransferCompleteChannel(transferID string) string {
	return DaemonFileTransferComplete + transferID
}

func BuildRealtimeConsoleResultChannel(serverID uint64) string {
	return RealtimeConsoleResult + strconv.FormatUint(serverID, 10)
}

func BuildDaemonCommandRequestChannel(nodeID uint64) string {
	return DaemonCommandRequest + strconv.FormatUint(nodeID, 10)
}

func BuildDaemonCommandResponseChannel(instanceID string) string {
	return DaemonCommandResponse + instanceID
}

func BuildDaemonStatusRequestChannel(nodeID uint64) string {
	return DaemonStatusRequest + strconv.FormatUint(nodeID, 10)
}

func BuildDaemonStatusResponseChannel(instanceID string) string {
	return DaemonStatusResponse + instanceID
}

func BuildRealtimeAttachStartedChannel(sessionID string) string {
	return RealtimeAttachStarted + sessionID
}

func BuildRealtimeAttachOutputChannel(sessionID string) string {
	return RealtimeAttachOutput + sessionID
}

func BuildRealtimeAttachClosedChannel(sessionID string) string {
	return RealtimeAttachClosed + sessionID
}

func BuildDaemonAttachDispatchChannel(nodeID uint64) string {
	return DaemonAttachDispatch + strconv.FormatUint(nodeID, 10)
}

func BuildDaemonConsoleLogRequestChannel(nodeID uint64) string {
	return DaemonConsoleLogRequest + strconv.FormatUint(nodeID, 10)
}

func BuildDaemonConsoleLogResponseChannel(instanceID string) string {
	return DaemonConsoleLogResponse + instanceID
}

func BuildDaemonHTTPProxyRequestChannel(nodeID uint64) string {
	return DaemonHTTPProxyRequest + strconv.FormatUint(nodeID, 10)
}

func BuildDaemonHTTPProxyResponseChannel(instanceID string) string {
	return DaemonHTTPProxyResponse + instanceID
}

func BuildRealtimeMetricsChannel(nodeID uint64) string {
	return RealtimeMetrics + strconv.FormatUint(nodeID, 10)
}

func BuildDaemonMetricsRequestChannel(nodeID uint64) string {
	return DaemonMetricsRequest + strconv.FormatUint(nodeID, 10)
}

func BuildDaemonMetricsResponseChannel(instanceID string) string {
	return DaemonMetricsResponse + instanceID
}

func BuildMetricsSubscribersChannel(nodeID uint64) string {
	return MetricsSubscribers + strconv.FormatUint(nodeID, 10)
}

func BuildDaemonServerTaskDeltaChannel(nodeID uint64) string {
	return DaemonServerTaskDelta + strconv.FormatUint(nodeID, 10)
}

func BuildDaemonServerTaskResyncChannel(nodeID uint64) string {
	return DaemonServerTaskResync + strconv.FormatUint(nodeID, 10)
}

func BuildRealtimeServerTaskExecutionChannel(taskID uint64) string {
	return RealtimeServerTaskExecution + strconv.FormatUint(taskID, 10)
}

func BuildRealtimeServerTaskLogChannel(executionID string) string {
	return RealtimeServerTaskLog + executionID
}

func BuildRealtimeArchiveOpChannel(operationID string) string {
	return RealtimeArchiveOp + operationID
}

func BuildRealtimeFMArchiveChannel(serverID uint64) string {
	return RealtimeFMArchive + strconv.FormatUint(serverID, 10)
}

func BuildDaemonArchiveRequestChannel(nodeID uint64) string {
	return DaemonArchiveRequest + strconv.FormatUint(nodeID, 10)
}

func BuildDaemonArchiveResponseChannel(instanceID string) string {
	return DaemonArchiveResponse + instanceID
}
