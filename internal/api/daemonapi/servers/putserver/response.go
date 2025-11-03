package putserver

type updateServerResponse struct {
	Message string `json:"message"`
}

func newUpdateServerResponse() *updateServerResponse {
	return &updateServerResponse{
		Message: "success",
	}
}
