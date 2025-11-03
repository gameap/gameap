package upload

type uploadResponse struct {
	Result resultResponse `json:"result"`
}

type resultResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func newUploadResponse() uploadResponse {
	return uploadResponse{
		Result: resultResponse{
			Status:  "success",
			Message: "All files uploaded!",
		},
	}
}
