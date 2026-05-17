package setup

type setupResponse struct {
	Secret     string `json:"secret"`
	OtpauthURI string `json:"otpauth_uri"`
}

func newSetupResponse(secret, otpauthURI string) setupResponse {
	return setupResponse{
		Secret:     secret,
		OtpauthURI: otpauthURI,
	}
}
