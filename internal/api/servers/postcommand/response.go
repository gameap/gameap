package postcommand

type commandResponse struct {
	DaemonTaskID uint `json:"gdaemonTaskId"`
}

func newCommandResponse(daemonTaskID uint) *commandResponse {
	return &commandResponse{
		DaemonTaskID: daemonTaskID,
	}
}
