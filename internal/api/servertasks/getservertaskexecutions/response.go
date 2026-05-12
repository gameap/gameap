package getservertaskexecutions

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/rs/xid"
)

type executionResponse struct {
	ID                uint       `json:"id"`
	ExecutionID       xid.ID     `json:"execution_id"`
	ServerTaskID      uint       `json:"server_task_id"`
	ServerID          uint       `json:"server_id"`
	NodeID            uint       `json:"node_id"`
	Command           string     `json:"command"`
	TaskVersion       uint64     `json:"task_version"`
	Status            string     `json:"status"`
	ExitCode          *int       `json:"exit_code,omitempty"`
	ErrorMessage      *string    `json:"error_message,omitempty"`
	StartedAt         time.Time  `json:"started_at"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	DurationMS        *int64     `json:"duration_ms,omitempty"`
	OutputInline      *string    `json:"output_inline,omitempty"`
	OutputStoragePath *string    `json:"output_storage_path,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type executionsResponse struct {
	Data []executionResponse `json:"data"`
}

func newExecutionsResponse(execs []domain.ServerTaskExecution, isAdmin bool) executionsResponse {
	out := make([]executionResponse, 0, len(execs))
	for i := range execs {
		e := &execs[i]
		resp := executionResponse{
			ID:           e.ID,
			ExecutionID:  e.ExecutionID,
			ServerTaskID: e.ServerTaskID,
			ServerID:     e.ServerID,
			NodeID:       e.NodeID,
			Command:      string(e.Command),
			TaskVersion:  e.TaskVersion,
			Status:       string(e.Status),
			ExitCode:     e.ExitCode,
			ErrorMessage: e.ErrorMessage,
			StartedAt:    e.StartedAt,
			FinishedAt:   e.FinishedAt,
			DurationMS:   e.DurationMS,
			CreatedAt:    e.CreatedAt,
			UpdatedAt:    e.UpdatedAt,
		}

		if isAdmin {
			resp.OutputInline = e.OutputInline
			resp.OutputStoragePath = e.OutputStoragePath
		}

		out = append(out, resp)
	}

	return executionsResponse{Data: out}
}
