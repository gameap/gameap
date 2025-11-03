package kickplayer

type kickResponse struct {
	Message string `json:"message"`
}

func newKickResponse(message string) kickResponse {
	return kickResponse{
		Message: message,
	}
}
