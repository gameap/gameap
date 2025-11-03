package postserver

type createServerResult struct {
	TaskID   uint `json:"taskId"`
	ServerID uint `json:"serverId"`
}

type createServerResponse struct {
	Message string             `json:"message"`
	Result  createServerResult `json:"result"`
}
