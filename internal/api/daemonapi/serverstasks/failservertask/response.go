package failservertask

type failServerTaskResponse struct {
	Message string `json:"message"`
}

func newFailServerTaskResponse() *failServerTaskResponse {
	return &failServerTaskResponse{
		Message: "success",
	}
}
