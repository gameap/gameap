package updateservertask

type updateServerTaskResponse struct {
	Message string `json:"message"`
}

func newUpdateServerTaskResponse() *updateServerTaskResponse {
	return &updateServerTaskResponse{
		Message: "success",
	}
}
