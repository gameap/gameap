package getsummary

type summaryResponse struct {
	Total   int `json:"total"`
	Online  int `json:"online"`
	Offline int `json:"offline"`
}
