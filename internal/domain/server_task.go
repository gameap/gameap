package domain

import "time"

type ServerTaskCommand string

const (
	ServerTaskCommandStart     ServerTaskCommand = "start"
	ServerTaskCommandStop      ServerTaskCommand = "stop"
	ServerTaskCommandRestart   ServerTaskCommand = "restart"
	ServerTaskCommandUpdate    ServerTaskCommand = "update"
	ServerTaskCommandReinstall ServerTaskCommand = "reinstall"
)

func NewServerTaskCommandFromString(s string) ServerTaskCommand {
	switch s {
	case "start":
		return ServerTaskCommandStart
	case "stop":
		return ServerTaskCommandStop
	case "restart":
		return ServerTaskCommandRestart
	case "update":
		return ServerTaskCommandUpdate
	case "reinstall":
		return ServerTaskCommandReinstall
	default:
		return ""
	}
}

type ServerTask struct {
	ID           uint              `db:"id"`
	Command      ServerTaskCommand `db:"command"`
	ServerID     uint              `db:"server_id"`
	Repeat       uint8             `db:"repeat"`
	RepeatPeriod time.Duration     `db:"repeat_period"`
	Counter      uint              `db:"counter"`
	ExecuteDate  time.Time         `db:"execute_date"`
	Payload      *string           `db:"payload"`
	CreatedAt    *time.Time        `db:"created_at"`
	UpdatedAt    *time.Time        `db:"updated_at"`
}

type ServerTaskFail struct {
	ID           uint       `db:"id"`
	ServerTaskID uint       `db:"server_task_id"`
	Output       string     `db:"output"`
	CreatedAt    *time.Time `db:"created_at"`
	UpdatedAt    *time.Time `db:"updated_at"`
}
