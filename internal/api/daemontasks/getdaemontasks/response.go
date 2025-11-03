package getdaemontasks

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type daemonTaskResponse struct {
	ID                uint                    `json:"id"`
	RunAftID          *uint                   `json:"run_aft_id,omitempty"`
	CreatedAt         *time.Time              `json:"created_at,omitempty"`
	UpdatedAt         *time.Time              `json:"updated_at,omitempty"`
	DedicatedServerID uint                    `json:"dedicated_server_id"`
	ServerID          *uint                   `json:"server_id,omitempty"`
	Task              domain.DaemonTaskType   `json:"task"`
	Cmd               *string                 `json:"cmd,omitempty"`
	Status            domain.DaemonTaskStatus `json:"status"`
}

func newDaemonTasksResponseFromDaemonTasks(tasks []domain.DaemonTask) []daemonTaskResponse {
	response := make([]daemonTaskResponse, 0, len(tasks))

	for _, task := range tasks {
		response = append(response, newDaemonTaskResponseFromDaemonTask(&task))
	}

	return response
}

func newDaemonTaskResponseFromDaemonTask(task *domain.DaemonTask) daemonTaskResponse {
	return daemonTaskResponse{
		ID:                task.ID,
		RunAftID:          task.RunAftID,
		CreatedAt:         task.CreatedAt,
		UpdatedAt:         task.UpdatedAt,
		DedicatedServerID: task.DedicatedServerID,
		ServerID:          task.ServerID,
		Task:              task.Task,
		Cmd:               task.Cmd,
		Status:            task.Status,
	}
}
