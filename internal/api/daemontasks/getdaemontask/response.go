package getdaemontask

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type daemonTaskOutputResponse struct {
	ID                uint                    `json:"id"`
	DedicatedServerID uint                    `json:"dedicated_server_id"`
	ServerID          *uint                   `json:"server_id"`
	Task              domain.DaemonTaskType   `json:"task"`
	CreatedAt         *time.Time              `json:"created_at"`
	UpdatedAt         *time.Time              `json:"updated_at"`
	Output            *string                 `json:"output,omitempty"`
	Status            domain.DaemonTaskStatus `json:"status"`
}

func newDaemonTaskOutputResponseFromDaemonTask(task *domain.DaemonTask) daemonTaskOutputResponse {
	return daemonTaskOutputResponse{
		ID:                task.ID,
		DedicatedServerID: task.DedicatedServerID,
		ServerID:          task.ServerID,
		Task:              task.Task,
		CreatedAt:         task.CreatedAt,
		UpdatedAt:         task.UpdatedAt,
		Output:            task.Output,
		Status:            task.Status,
	}
}
