package initialize

type initializeResponse struct {
	Result resultResponse `json:"result"`
	Config configResponse `json:"config"`
}

type resultResponse struct {
	Status  string  `json:"status"`
	Message *string `json:"message"`
}

type configResponse struct {
	LeftDisk      *string                 `json:"leftDisk"`
	RightDisk     *string                 `json:"rightDisk"`
	WindowsConfig int                     `json:"windowsConfig"`
	Disks         map[string]diskResponse `json:"disks"`
	Lang          string                  `json:"lang"`
}

type diskResponse struct {
	Driver string `json:"driver"`
}

func newInitializeResponse() initializeResponse {
	return initializeResponse{
		Result: resultResponse{
			Status:  "success",
			Message: nil,
		},
		Config: configResponse{
			LeftDisk:      nil,
			RightDisk:     nil,
			WindowsConfig: 1,
			Disks: map[string]diskResponse{
				"server": {
					Driver: "gameap",
				},
			},
			Lang: "",
		},
	}
}
