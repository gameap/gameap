package appendoutput

type appendOutputResponse struct {
	Message string `json:"message"`
}

func newAppendOutputResponse() *appendOutputResponse {
	return &appendOutputResponse{
		Message: "success",
	}
}
