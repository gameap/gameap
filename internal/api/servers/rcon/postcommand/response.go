package postcommand

type commandResponse struct {
	Output string `json:"output"`
}

func newCommandResponse(output string) commandResponse {
	return commandResponse{
		Output: output,
	}
}
