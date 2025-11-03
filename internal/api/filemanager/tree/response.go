package tree

type treeResponse struct {
	Result      resultResponse      `json:"result"`
	Directories []directoryTreeItem `json:"directories"`
}

type resultResponse struct {
	Status  string  `json:"status"`
	Message *string `json:"message"`
}

type directoryTreeItem struct {
	Path      string         `json:"path"`
	Timestamp uint64         `json:"timestamp"`
	Type      string         `json:"type"`
	Dirname   string         `json:"dirname"`
	Basename  string         `json:"basename"`
	Props     directoryProps `json:"props"`
}

type directoryProps struct {
	HasSubdirectories bool `json:"hasSubdirectories"`
}

func newTreeResponse(directories []directoryTreeItem) treeResponse {
	return treeResponse{
		Result: resultResponse{
			Status:  "success",
			Message: nil,
		},
		Directories: directories,
	}
}
