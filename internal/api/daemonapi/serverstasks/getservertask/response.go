package getservertask

import (
	"github.com/gameap/gameap/internal/domain"
)

type ServerTaskResponse struct {
	ID           uint    `json:"id"`
	Command      string  `json:"command"`
	ServerID     uint    `json:"server_id"`
	Repeat       uint8   `json:"repeat"`
	RepeatPeriod int     `json:"repeat_period"`
	Counter      uint    `json:"counter"`
	ExecuteDate  string  `json:"execute_date"`
	Payload      *string `json:"payload"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

func newServerTaskResponse(task *domain.ServerTask) ServerTaskResponse {
	var createdAt string
	if task.CreatedAt != nil {
		createdAt = task.CreatedAt.Format("2006-01-02T15:04:05.000000Z")
	}

	var updatedAt string
	if task.UpdatedAt != nil {
		updatedAt = task.UpdatedAt.Format("2006-01-02T15:04:05.000000Z")
	}

	executeDate := task.ExecuteDate.Format("2006-01-02 15:04:05")

	return ServerTaskResponse{
		ID:           task.ID,
		Command:      string(task.Command),
		ServerID:     task.ServerID,
		Repeat:       task.Repeat,
		RepeatPeriod: int(task.RepeatPeriod.Seconds()),
		Counter:      task.Counter,
		ExecuteDate:  executeDate,
		Payload:      task.Payload,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}
}
