package postserver

type createServerResult struct {
	TaskID     uint   `json:"taskId"`
	ServerID   uint   `json:"serverId"`
	ServerIP   string `json:"serverIp"`
	ServerPort int    `json:"serverPort"`
	QueryPort  *int   `json:"queryPort"`
	RconPort   *int   `json:"rconPort"`
}

type createServerResponse struct {
	Message string             `json:"message"`
	Result  createServerResult `json:"result"`
}
