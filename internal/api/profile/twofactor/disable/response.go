package disable

type disableResponse struct {
	Status string `json:"status"`
}

func newDisableResponse() disableResponse {
	return disableResponse{Status: "ok"}
}
