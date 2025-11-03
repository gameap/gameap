package patchservers

type bulkUpdateServerResponse struct {
	Message string `json:"message"`
}

func newBulkUpdateServerResponse() *bulkUpdateServerResponse {
	return &bulkUpdateServerResponse{
		Message: "success",
	}
}
