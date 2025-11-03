package deletefiles

type deleteRequest struct {
	Disk  string       `json:"disk"`
	Items []deleteItem `json:"items"`
}

type deleteItem struct {
	Path string `json:"path"`
	Type string `json:"type"`
}
