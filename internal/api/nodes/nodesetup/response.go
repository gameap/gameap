package nodesetup

type setupResponse struct {
	Link  string `json:"link"`
	Token string `json:"token"`
	Host  string `json:"host"`

	GRPCEnabled bool   `json:"grpc_enabled"`
	ConnectURL  string `json:"connect_url,omitempty"`
	LinuxCmd    string `json:"linux_cmd,omitempty"`
	WindowsCmd  string `json:"windows_cmd,omitempty"`
	SetupLink   string `json:"setup_link,omitempty"`

	Warnings []string `json:"warnings,omitempty"`
}
