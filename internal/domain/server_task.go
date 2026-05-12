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

type ServerTaskOverlapPolicy string

const (
	ServerTaskOverlapPolicySkip  ServerTaskOverlapPolicy = "skip"
	ServerTaskOverlapPolicyQueue ServerTaskOverlapPolicy = "queue"
)

func NewServerTaskOverlapPolicyFromString(s string) ServerTaskOverlapPolicy {
	switch s {
	case "skip":
		return ServerTaskOverlapPolicySkip
	case "queue":
		return ServerTaskOverlapPolicyQueue
	default:
		return ServerTaskOverlapPolicySkip
	}
}

type ServerTaskCatchupPolicy string

const (
	ServerTaskCatchupPolicySkip    ServerTaskCatchupPolicy = "skip"
	ServerTaskCatchupPolicyRunOnce ServerTaskCatchupPolicy = "run_once"
)

func NewServerTaskCatchupPolicyFromString(s string) ServerTaskCatchupPolicy {
	switch s {
	case "skip":
		return ServerTaskCatchupPolicySkip
	case "run_once":
		return ServerTaskCatchupPolicyRunOnce
	default:
		return ServerTaskCatchupPolicySkip
	}
}

type ServerTask struct {
	ID              uint                    `db:"id"`
	Command         ServerTaskCommand       `db:"command"`
	ServerID        uint                    `db:"server_id"`
	NodeID          *uint                   `db:"node_id"`
	Name            *string                 `db:"name"`
	Enabled         bool                    `db:"enabled"`
	OverlapPolicy   ServerTaskOverlapPolicy `db:"overlap_policy"`
	CatchupPolicy   ServerTaskCatchupPolicy `db:"catchup_policy"`
	CreatedByUserID *uint                   `db:"created_by_user_id"`
	Version         uint64                  `db:"version"`
	Timezone        *string                 `db:"timezone"`
	Repeat          uint8                   `db:"repeat"`
	RepeatPeriod    time.Duration           `db:"repeat_period"`
	Counter         uint                    `db:"counter"`
	ExecuteDate     time.Time               `db:"execute_date"`
	Payload         *string                 `db:"payload"`
	CreatedAt       *time.Time              `db:"created_at"`
	UpdatedAt       *time.Time              `db:"updated_at"`
	DeletedAt       *time.Time              `db:"deleted_at"`
}
