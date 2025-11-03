package rename

type renameResponse struct {
	Result resultResponse `json:"result"`
}

type resultResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func newRenameResponse() renameResponse {
	return renameResponse{
		Result: resultResponse{
			Status:  "success",
			Message: "Renamed!",
		},
	}
}
