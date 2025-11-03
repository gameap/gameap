package gettoken

type tokenResponse struct {
	Token     string `json:"token"`
	Timestamp int64  `json:"timestamp"`
}

func newTokenResponse(token string, timestamp int64) tokenResponse {
	return tokenResponse{
		Token:     token,
		Timestamp: timestamp,
	}
}
