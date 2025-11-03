package rename

type renameRequest struct {
	Disk    string `json:"disk"`
	OldName string `json:"oldName"`
	NewName string `json:"newName"`
	Type    string `json:"type"`
}
