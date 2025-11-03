package domain

import "time"

type DaemonTaskStatus string

const (
	DaemonTaskStatusWaiting  DaemonTaskStatus = "waiting"
	DaemonTaskStatusWorking  DaemonTaskStatus = "working"
	DaemonTaskStatusError    DaemonTaskStatus = "error"
	DaemonTaskStatusSuccess  DaemonTaskStatus = "success"
	DaemonTaskStatusCanceled DaemonTaskStatus = "canceled"
)

var DaemonTaskStatusNums = map[DaemonTaskStatus]int{
	DaemonTaskStatusWaiting:  1,
	DaemonTaskStatusWorking:  2,
	DaemonTaskStatusError:    3,
	DaemonTaskStatusSuccess:  4,
	DaemonTaskStatusCanceled: 5,
}

type DaemonTaskType string

const (
	DaemonTaskTypeServerStart   DaemonTaskType = "gsstart"
	DaemonTaskTypeServerStop    DaemonTaskType = "gsstop"
	DaemonTaskTypeServerRestart DaemonTaskType = "gsrest"
	DaemonTaskTypeServerUpdate  DaemonTaskType = "gsupd"
	DaemonTaskTypeServerInstall DaemonTaskType = "gsinst"
	DaemonTaskTypeServerDelete  DaemonTaskType = "gsdel"
	DaemonTaskTypeServerMove    DaemonTaskType = "gsmove"
	DaemonTaskTypeCmdExec       DaemonTaskType = "cmdexec"
)

type DaemonTask struct {
	ID                uint             `db:"id"`
	RunAftID          *uint            `db:"run_aft_id"`
	CreatedAt         *time.Time       `db:"created_at"`
	UpdatedAt         *time.Time       `db:"updated_at"`
	DedicatedServerID uint             `db:"dedicated_server_id"`
	ServerID          *uint            `db:"server_id"`
	Task              DaemonTaskType   `db:"task"`
	Data              *string          `db:"data"`
	Cmd               *string          `db:"cmd"`
	Output            *string          `db:"output"`
	Status            DaemonTaskStatus `db:"status"`
}
