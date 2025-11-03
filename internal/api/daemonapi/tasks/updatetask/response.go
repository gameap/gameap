package updatetask

type updateTaskResponse struct {
	Message string `json:"message"`
}

func newUpdateTaskResponse() *updateTaskResponse {
	return &updateTaskResponse{
		Message: "success",
	}
}
