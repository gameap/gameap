package getdaemonstatus

import (
	"strconv"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
)

type versionInfo struct {
	Version     string `json:"version"`
	CompileDate string `json:"compile_date"`
}

type baseInfo struct {
	Uptime             string `json:"uptime"`
	WorkingTasksCount  string `json:"working_tasks_count"`
	WaitingTasksCount  string `json:"waiting_tasks_count"`
	OnlineServersCount string `json:"online_servers_count"`
}

type daemonStatusResponse struct {
	ID       uint        `json:"id"`
	Name     string      `json:"name"`
	APIKey   string      `json:"api_key"`
	Version  versionInfo `json:"version"`
	BaseInfo baseInfo    `json:"base_info"`
}

func newDaemonStatusResponse(node *domain.Node, status *daemon.NodeStatus) daemonStatusResponse {
	return daemonStatusResponse{
		ID:     node.ID,
		Name:   node.Name,
		APIKey: node.GdaemonAPIKey,
		Version: versionInfo{
			Version:     status.Version,
			CompileDate: status.BuildDate,
		},
		BaseInfo: baseInfo{
			Uptime:             status.Uptime.String(),
			WorkingTasksCount:  strconv.Itoa(status.WorkingTasks),
			WaitingTasksCount:  strconv.Itoa(status.WaitingTasks),
			OnlineServersCount: strconv.Itoa(status.OnlineServers),
		},
	}
}
