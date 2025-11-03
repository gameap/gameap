package getconsole

type consoleResponse struct {
	Console string `json:"console"`
}

func newConsoleResponse(console string) consoleResponse {
	return consoleResponse{
		Console: console,
	}
}
