package getsummary

type nodeSummary struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Location  string `json:"location"`
	Enabled   bool   `json:"enabled"`
	Online    bool   `json:"online"`
	Version   string `json:"version,omitempty"`
	BuildDate string `json:"buildDate,omitempty"`
	// Outdated is set when the daemon runs a version older than the latest
	// stable gameap-daemon release. It stays absent when the update check is
	// disabled or the latest release could not be resolved.
	Outdated bool `json:"outdated,omitempty"`
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
