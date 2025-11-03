package paste

type pasteResponse struct {
	Result resultResponse `json:"result"`
}

type resultResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func newPasteResponse(operationType string) pasteResponse {
	message := "Copied successfully!"
	if operationType == operationTypeCut {
		message = "Moved successfully!"
	}

	return pasteResponse{
		Result: resultResponse{
			Status:  "success",
			Message: message,
		},
	}
}
