package nodesetup

type setupResponse struct {
	Link  string `json:"link"`
	Token string `json:"token"`
	Host  string `json:"host"`
}

func newSetupResponse(token string, baseURL string) setupResponse {
	return setupResponse{
		Link:  baseURL + "/gdaemon/setup/" + token,
		Token: token,
		Host:  baseURL,
	}
}
