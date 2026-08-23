package messages

import (
	"encoding/json"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/pkg/idgen"
)

const (
	TypeCacheInvalidate = "cache.invalidate"
	TypePluginEvent     = "plugin.event"
	TypePluginSync      = "plugin.sync"
	TypeServerStatus    = "server.status"
	TypeTaskProgress    = "task.progress"
	TypeNotification    = "notification"

	TypeDaemonConnected    = "daemon.connected"
	TypeDaemonClosed       = "daemon.closed"
	TypeDaemonTask         = "daemon.task"
	TypeDaemonCommand      = "daemon.command"
	TypeDaemonServerConfig = "daemon.server_config"

	TypeTaskStatus    = "task.status"
	TypeTaskOutput    = "task.output"
	TypeTaskComplete  = "task.complete"
	TypeConsoleOutput = "console.output"
	TypeConsoleResult = "console.result"

	TypeDaemonFileRequest          = "daemon.file.request"
	TypeDaemonFileResponse         = "daemon.file.response"
	TypeDaemonFileTransferComplete = "daemon.file.transfer.complete"

	TypeDaemonCommandRequest  = "daemon.command.request"
	TypeDaemonCommandResponse = "daemon.command.response"

	TypeDaemonStatusRequest  = "daemon.status.request"
	TypeDaemonStatusResponse = "daemon.status.response"

	TypeDaemonConsoleLogRequest  = "daemon.consolelog.request"
	TypeDaemonConsoleLogResponse = "daemon.consolelog.response"

	TypeAttachStarted = "attach.started"
	TypeAttachOutput  = "attach.output"
	TypeAttachClosed  = "attach.closed"
	TypeDaemonAttach  = "daemon.attach"

	TypeDaemonHTTPProxyRequest  = "daemon.httpproxy.request"
	TypeDaemonHTTPProxyResponse = "daemon.httpproxy.response"

	TypeMetricsLive           = "metrics.live"
	TypeDaemonMetricsRequest  = "daemon.metrics.request"
	TypeDaemonMetricsResponse = "daemon.metrics.response"
	TypeMetricsSubscribers    = "metrics.subscribers"

	TypeDaemonServerTaskDelta       = "daemon.server_task.delta"
	TypeDaemonServerTaskResync      = "daemon.server_task.resync"
	TypeServerTaskExecutionStarted  = "server_task.execution.started"
	TypeServerTaskExecutionFinished = "server_task.execution.finished"
	TypeServerTaskExecutionLog      = "server_task.execution.log"

	TypeArchiveProgress       = "archive.progress"
	TypeArchiveComplete       = "archive.complete"
	TypeDaemonArchiveRequest  = "daemon.archive.request"
	TypeDaemonArchiveResponse = "daemon.archive.response"
)

type CacheInvalidatePayload struct {
	EntityType string   `json:"entity_type"`
	EntityIDs  []string `json:"entity_ids,omitempty"`
	Pattern    string   `json:"pattern,omitempty"`
}

// PluginSyncPayload is a hint, not state. The receiving instance always
// re-reads the plugins table before it decides anything, so a message that is
// lost, duplicated or delivered out of order costs at most one refresh
// interval of staleness. The fields exist for logging.
type PluginSyncPayload struct {
	PluginID uint64 `json:"plugin_id,omitempty"`
	Action   string `json:"action,omitempty"`
}

type PluginEventPayload struct {
	EventType int32             `json:"event_type"`
	ServerID  *uint             `json:"server_id,omitempty"`
	TaskID    *uint             `json:"task_id,omitempty"`
	NodeID    *uint             `json:"node_id,omitempty"`
	ExtraData map[string]string `json:"extra_data,omitempty"`
}

type ServerStatusPayload struct {
	ServerID      uint   `json:"server_id"`
	Status        string `json:"status"`
	PlayersOnline int    `json:"players_online"`
	MaxPlayers    int    `json:"max_players"`
}

type TaskProgressPayload struct {
	TaskID   uint   `json:"task_id"`
	ServerID *uint  `json:"server_id,omitempty"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Message  string `json:"message,omitempty"`
}

type DaemonSessionPayload struct {
	NodeID      uint64    `json:"node_id"`
	InstanceID  string    `json:"instance_id"`
	Version     string    `json:"version"`
	ConnectedAt time.Time `json:"connected_at"`
}

type DaemonTaskDispatchPayload struct {
	NodeID    uint64 `json:"node_id"`
	RequestID string `json:"request_id"`
	TaskID    uint64 `json:"task_id"`
	TaskData  []byte `json:"task_data"`
}

type DaemonCommandDispatchPayload struct {
	NodeID    uint64 `json:"node_id"`
	RequestID string `json:"request_id"`
	CommandID string `json:"command_id"`
	ServerID  uint64 `json:"server_id"`
	Command   string `json:"command"`
	Timeout   int32  `json:"timeout"`
}

type TaskStatusPayload struct {
	TaskID   uint64 `json:"task_id"`
	Status   string `json:"status"`
	ServerID uint   `json:"server_id"`
	Message  string `json:"message,omitempty"`
}

type TaskOutputPayload struct {
	TaskID  uint64 `json:"task_id"`
	Chunk   string `json:"chunk"`
	IsFinal bool   `json:"is_final"`
}

type TaskCompletePayload struct {
	TaskID   uint64 `json:"task_id"`
	Status   string `json:"status"`
	ServerID uint   `json:"server_id"`
}

type ConsoleOutputPayload struct {
	ServerID  uint64 `json:"server_id"`
	CommandID string `json:"command_id,omitempty"`
	Chunk     string `json:"chunk"`
}

type ConsoleResultPayload struct {
	ServerID  uint64 `json:"server_id"`
	CommandID string `json:"command_id,omitempty"`
	ExitCode  int32  `json:"exit_code"`
	Error     string `json:"error,omitempty"`
}

type DaemonFileRequestPayload struct {
	NodeID      uint64 `json:"node_id"`
	RequestID   string `json:"request_id"`
	InstanceID  string `json:"instance_id"`
	Operation   string `json:"operation"`
	Data        []byte `json:"data,omitempty"`
	TransferID  string `json:"transfer_id,omitempty"`
	StoragePath string `json:"storage_path,omitempty"`
	OwnerUser   string `json:"owner_user,omitempty"`
	OwnerUID    int32  `json:"owner_uid,omitempty"`
	OwnerGID    int32  `json:"owner_gid,omitempty"`
	Mode        int32  `json:"mode,omitempty"`
	// TimeoutSeconds overrides the owning instance's execution timeout for
	// slow operations (e.g. hashing); capped by the owning side. 0 = default.
	TimeoutSeconds int64 `json:"timeout_seconds,omitempty"`
}

type DaemonFileResponsePayload struct {
	RequestID   string `json:"request_id"`
	Error       string `json:"error,omitempty"`
	Data        []byte `json:"data,omitempty"`
	StoragePath string `json:"storage_path,omitempty"`
}

type FileTransferCompletePayload struct {
	TransferID string `json:"transfer_id"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	Checksum   string `json:"checksum,omitempty"`
}

type DaemonCommandRequestPayload struct {
	NodeID     uint64 `json:"node_id"`
	RequestID  string `json:"request_id"`
	InstanceID string `json:"instance_id"`
	Data       []byte `json:"data,omitempty"`
}

type DaemonCommandResponsePayload struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error,omitempty"`
	Data      []byte `json:"data,omitempty"`
}

type DaemonStatusRequestPayload struct {
	NodeID     uint64 `json:"node_id"`
	RequestID  string `json:"request_id"`
	InstanceID string `json:"instance_id"`
}

type DaemonStatusResponsePayload struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error,omitempty"`
	Data      []byte `json:"data,omitempty"`
}

type AttachStartedPayload struct {
	SessionID string `json:"session_id"`
	ServerID  uint64 `json:"server_id"`
}

type AttachOutputPayload struct {
	SessionID string `json:"session_id"`
	Data      []byte `json:"data"`
}

type AttachClosedPayload struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason"`
	ExitCode  int32  `json:"exit_code"`
}

type DaemonAttachDispatchPayload struct {
	NodeID    uint64 `json:"node_id"`
	RequestID string `json:"request_id"`
	Data      []byte `json:"data"`
}

type DaemonConsoleLogRequestPayload struct {
	NodeID     uint64 `json:"node_id"`
	RequestID  string `json:"request_id"`
	InstanceID string `json:"instance_id"`
	ServerID   uint64 `json:"server_id"`
	MaxBytes   int64  `json:"max_bytes"`
}

type DaemonConsoleLogResponsePayload struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error,omitempty"`
	Data      []byte `json:"data,omitempty"`
}

type DaemonServerConfigPayload struct {
	NodeID     uint64 `json:"node_id"`
	RequestID  string `json:"request_id"`
	ConfigData []byte `json:"config_data"`
}

type DaemonHTTPProxyRequestPayload struct {
	NodeID     uint64 `json:"node_id"`
	RequestID  string `json:"request_id"`
	InstanceID string `json:"instance_id"`
	Data       []byte `json:"data,omitempty"`
}

type DaemonHTTPProxyResponsePayload struct {
	RequestID   string `json:"request_id"`
	Error       string `json:"error,omitempty"`
	Data        []byte `json:"data,omitempty"`
	StoragePath string `json:"storage_path,omitempty"`
}

type MetricsLivePayload struct {
	NodeID uint64 `json:"node_id"`
	Data   []byte `json:"data"`
}

type DaemonMetricsRequestPayload struct {
	NodeID     uint64 `json:"node_id"`
	RequestID  string `json:"request_id"`
	InstanceID string `json:"instance_id"`
	Data       []byte `json:"data"`
}

type DaemonMetricsResponsePayload struct {
	RequestID string `json:"request_id"`
	NodeID    uint64 `json:"node_id"`
	Error     string `json:"error,omitempty"`
	Data      []byte `json:"data,omitempty"`
}

type MetricsSubscribersPayload struct {
	InstanceID string    `json:"instance_id"`
	NodeID     uint64    `json:"node_id"`
	Count      int       `json:"count"`
	Timestamp  time.Time `json:"timestamp"`
}

// DaemonServerTaskDeltaPayload routes a serialised GatewayMessage that
// carries either a ServerTaskDelta.upserted or .deleted arm. The owning
// API instance for the daemon's bidi session forwards it to the stream.
type DaemonServerTaskDeltaPayload struct {
	NodeID    uint64 `json:"node_id"`
	RequestID string `json:"request_id"`
	TaskID    uint64 `json:"task_id"`
	Data      []byte `json:"data"`
}

// DaemonServerTaskResyncPayload is the daemon-initiated resync trigger
// observed by the session-owning instance; it relays a snapshot back
// over the bidi stream.
type DaemonServerTaskResyncPayload struct {
	NodeID           uint64 `json:"node_id"`
	LastKnownVersion uint64 `json:"last_known_version"`
}

type ServerTaskExecutionStatusPayload struct {
	ExecutionID  string `json:"execution_id"`
	TaskID       uint64 `json:"task_id"`
	ServerID     uint64 `json:"server_id"`
	NodeID       uint64 `json:"node_id"`
	Status       string `json:"status"`
	ExitCode     *int32 `json:"exit_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type ServerTaskExecutionLogPayload struct {
	ExecutionID string `json:"execution_id"`
	TaskID      uint64 `json:"task_id"`
	Sequence    uint64 `json:"sequence"`
	Chunk       []byte `json:"chunk"`
	IsFinal     bool   `json:"is_final"`
}

type ArchiveProgressEventPayload struct {
	OperationID    string `json:"operation_id"`
	NodeID         uint64 `json:"node_id"`
	ServerID       uint   `json:"server_id,omitempty"`
	Kind           string `json:"kind"`
	FilesProcessed uint32 `json:"files_processed"`
	FilesTotal     uint32 `json:"files_total,omitempty"`
	BytesProcessed uint64 `json:"bytes_processed"`
	BytesTotal     uint64 `json:"bytes_total,omitempty"`
	CurrentEntry   string `json:"current_entry,omitempty"`
}

type ArchiveCompleteEventPayload struct {
	OperationID    string   `json:"operation_id"`
	NodeID         uint64   `json:"node_id"`
	ServerID       uint     `json:"server_id,omitempty"`
	Kind           string   `json:"kind"`
	Success        bool     `json:"success"`
	Error          string   `json:"error,omitempty"`
	FilesProcessed uint32   `json:"files_processed"`
	BytesProcessed uint64   `json:"bytes_processed"`
	ArchiveSize    uint64   `json:"archive_size,omitempty"`
	Skipped        []string `json:"skipped,omitempty"`
	SkippedCount   uint32   `json:"skipped_count,omitempty"`
	Format         string   `json:"format,omitempty"`
}

type DaemonArchiveRequestPayload struct {
	NodeID     uint64 `json:"node_id"`
	RequestID  string `json:"request_id"`
	InstanceID string `json:"instance_id"`
	Action     string `json:"action"`
	ServerID   uint   `json:"server_id,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Initiator  string `json:"initiator,omitempty"`
	Data       []byte `json:"data,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// DaemonArchiveResponsePayload acknowledges that the session-owning instance
// accepted or rejected an archive start/cancel dispatch; it never carries the
// operation result, which arrives as an ArchiveCompleteEventPayload.
type DaemonArchiveResponsePayload struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error,omitempty"`
}

func NewMessage(channel, msgType string, payload any) (*pubsub.Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &pubsub.Message{
		ID:        idgen.New(),
		Channel:   channel,
		Type:      msgType,
		Payload:   data,
		Timestamp: time.Now(),
	}, nil
}

func ParsePayload[T any](msg *pubsub.Message) (T, error) {
	var payload T

	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return payload, err
	}

	return payload, nil
}
