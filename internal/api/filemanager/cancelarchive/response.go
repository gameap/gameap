package cancelarchive

type cancelArchiveResponse struct {
	Result resultResponse `json:"result"`
}

type resultResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func newCancelArchiveResponse() cancelArchiveResponse {
	return cancelArchiveResponse{
		Result: resultResponse{
			Status:  "success",
			Message: "Cancellation requested",
		},
	}
}
