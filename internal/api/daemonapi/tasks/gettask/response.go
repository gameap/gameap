package gettask

import (
	"github.com/gameap/gameap/internal/domain"
)

type TaskResponse struct {
	ID                uint   `json:"id"`
	RunAftID          *uint  `json:"run_after_id"`
	DedicatedServerID uint   `json:"dedicated_server_id"`
	ServerID          *uint  `json:"server_id"`
	Task              string `json:"task"`
	Data              string `json:"data"`
	Cmd               string `json:"cmd"`
	Status            string `json:"status"`
	StatusNum         int    `json:"status_num"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

func newTaskResponse(task *domain.DaemonTask) TaskResponse {
	var data string
	if task.Data != nil {
		data = *task.Data
	}

	var cmd string
	if task.Cmd != nil {
		cmd = *task.Cmd
	}

	var createdAt string
	if task.CreatedAt != nil {
		createdAt = task.CreatedAt.Format("2006-01-02T15:04:05.000000Z")
	}

	var updatedAt string
	if task.UpdatedAt != nil {
		updatedAt = task.UpdatedAt.Format("2006-01-02T15:04:05.000000Z")
	}

	return TaskResponse{
		ID:                task.ID,
		RunAftID:          task.RunAftID,
		DedicatedServerID: task.DedicatedServerID,
		ServerID:          task.ServerID,
		Task:              string(task.Task),
		Data:              data,
		Cmd:               cmd,
		Status:            string(task.Status),
		StatusNum:         getStatusNum(task.Status),
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
	}
}

func getStatusNum(status domain.DaemonTaskStatus) int {
	if num, ok := domain.DaemonTaskStatusNums[status]; ok {
		return num
	}

	return 0
}
