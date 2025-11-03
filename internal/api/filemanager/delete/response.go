package deletefiles

type deleteResponse struct {
	Result resultResponse `json:"result"`
}

type resultResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func newDeleteResponse() deleteResponse {
	return deleteResponse{
		Result: resultResponse{
			Status:  "success",
			Message: "Deleted!",
		},
	}
}
