package domain

import (
	"time"

	"github.com/rs/xid"
)

type ServerTaskExecutionStatus string

const (
	ServerTaskExecutionStatusRunning  ServerTaskExecutionStatus = "running"
	ServerTaskExecutionStatusSuccess  ServerTaskExecutionStatus = "success"
	ServerTaskExecutionStatusFailed   ServerTaskExecutionStatus = "failed"
	ServerTaskExecutionStatusCanceled ServerTaskExecutionStatus = "canceled"
	ServerTaskExecutionStatusSkipped  ServerTaskExecutionStatus = "skipped"
	ServerTaskExecutionStatusTimedOut ServerTaskExecutionStatus = "timed_out"
)

// ExecutionAbandonedReasonDaemonRestart marks executions reconciled at
// daemon (re)connect: the daemon dropped them between sessions and they
// will not resume.
const ExecutionAbandonedReasonDaemonRestart = "daemon_restart"

type ServerTaskExecution struct {
	ID                uint                      `db:"id"`
	ExecutionID       xid.ID                    `db:"execution_id"`
	ServerTaskID      uint                      `db:"server_task_id"`
	ServerID          uint                      `db:"server_id"`
	NodeID            uint                      `db:"node_id"`
	Command           ServerTaskCommand         `db:"command"`
	TaskVersion       uint64                    `db:"task_version"`
	Status            ServerTaskExecutionStatus `db:"status"`
	ExitCode          *int                      `db:"exit_code"`
	ErrorMessage      *string                   `db:"error_message"`
	StartedAt         time.Time                 `db:"started_at"`
	FinishedAt        *time.Time                `db:"finished_at"`
	DurationMS        *int64                    `db:"duration_ms"`
	OutputInline      *string                   `db:"output_inline"`
	OutputStoragePath *string                   `db:"output_storage_path"`
	CreatedAt         time.Time                 `db:"created_at"`
	UpdatedAt         time.Time                 `db:"updated_at"`
}
