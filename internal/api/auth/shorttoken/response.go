package shorttoken

type tokenResponse struct {
	Token     string `json:"token"`
	ExpiresIn int64  `json:"expires_in"` // Token lifetime in seconds
}

func newTokenResponse(token string, expiresIn int64) tokenResponse {
	return tokenResponse{
		Token:     token,
		ExpiresIn: expiresIn,
	}
}
