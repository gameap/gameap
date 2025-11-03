package getservertasks

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/carbon"
)

type serverTaskResponse struct {
	ID           uint       `json:"id"`
	Command      string     `json:"command"`
	ServerID     uint       `json:"server_id"`
	Repeat       uint8      `json:"repeat"`
	RepeatPeriod string     `json:"repeat_period"`
	Counter      uint       `json:"counter"`
	ExecuteDate  time.Time  `json:"execute_date"`
	Payload      *string    `json:"payload"`
	CreatedAt    *time.Time `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at"`
}

func newServerTasksResponseFromServerTasks(tasks []domain.ServerTask) []serverTaskResponse {
	response := make([]serverTaskResponse, 0, len(tasks))

	for _, task := range tasks {
		response = append(response, newServerTaskResponseFromServerTask(&task))
	}

	return response
}

func newServerTaskResponseFromServerTask(task *domain.ServerTask) serverTaskResponse {
	return serverTaskResponse{
		ID:           task.ID,
		Command:      string(task.Command),
		ServerID:     task.ServerID,
		Repeat:       task.Repeat,
		RepeatPeriod: carbon.Humanize(task.RepeatPeriod * time.Second),
		Counter:      task.Counter,
		ExecuteDate:  task.ExecuteDate,
		Payload:      task.Payload,
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
	}
}
