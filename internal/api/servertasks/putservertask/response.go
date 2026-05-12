package putservertask

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/carbon"
)

type serverTaskResponse struct {
	ID            uint       `json:"id"`
	Command       string     `json:"command"`
	ServerID      uint       `json:"server_id"`
	NodeID        *uint      `json:"node_id"`
	Name          *string    `json:"name"`
	Enabled       bool       `json:"enabled"`
	OverlapPolicy string     `json:"overlap_policy"`
	CatchupPolicy string     `json:"catchup_policy"`
	Timezone      *string    `json:"timezone"`
	Version       uint64     `json:"version"`
	Repeat        uint8      `json:"repeat"`
	RepeatPeriod  string     `json:"repeat_period"`
	Counter       uint       `json:"counter"`
	ExecuteDate   time.Time  `json:"execute_date"`
	Payload       *string    `json:"payload"`
	CreatedAt     *time.Time `json:"created_at"`
	UpdatedAt     *time.Time `json:"updated_at"`
}

func newServerTaskResponseFromServerTask(task *domain.ServerTask) serverTaskResponse {
	return serverTaskResponse{
		ID:            task.ID,
		Command:       string(task.Command),
		ServerID:      task.ServerID,
		NodeID:        task.NodeID,
		Name:          task.Name,
		Enabled:       task.Enabled,
		OverlapPolicy: string(task.OverlapPolicy),
		CatchupPolicy: string(task.CatchupPolicy),
		Timezone:      task.Timezone,
		Version:       task.Version,
		Repeat:        task.Repeat,
		RepeatPeriod:  carbon.Humanize(task.RepeatPeriod),
		Counter:       task.Counter,
		ExecuteDate:   task.ExecuteDate,
		Payload:       task.Payload,
		CreatedAt:     task.CreatedAt,
		UpdatedAt:     task.UpdatedAt,
	}
}
