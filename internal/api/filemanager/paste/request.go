package paste

type pasteRequest struct {
	Disk      string    `json:"disk"`
	Path      string    `json:"path"`
	Clipboard clipboard `json:"clipboard"`
}

type clipboard struct {
	Type        string   `json:"type"`
	Disk        string   `json:"disk"`
	Directories []string `json:"directories"`
	Files       []string `json:"files"`
}
