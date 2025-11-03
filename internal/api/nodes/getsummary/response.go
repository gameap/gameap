package getsummary

type nodeSummary struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Location  string `json:"location"`
	Enabled   bool   `json:"enabled"`
	Online    bool   `json:"online"`
	Version   string `json:"version,omitempty"`
	BuildDate string `json:"buildDate,omitempty"`
}

type summaryResponse struct {
	Total        int           `json:"total"`
	Enabled      int           `json:"enabled"`
	Disabled     int           `json:"disabled"`
	Online       int           `json:"online"`
	Offline      int           `json:"offline"`
	OnlineNodes  []nodeSummary `json:"onlineNodes"`
	OfflineNodes []nodeSummary `json:"offlineNodes"`
}
