package postconsole

type consoleResponse struct {
	Message string `json:"message"`
}

func newConsoleResponse() consoleResponse {
	return consoleResponse{
		Message: "success",
	}
}
