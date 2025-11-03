package createdirectory

import (
	"path/filepath"

	"github.com/gameap/gameap/internal/daemon"
)

type createDirectoryResponse struct {
	Result    resultResponse        `json:"result"`
	Directory directoryItemResponse `json:"directory"`
	Tree      []treeItemResponse    `json:"tree"`
}

type resultResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type directoryItemResponse struct {
	Path       string `json:"path"`
	Size       uint64 `json:"size"`
	Type       string `json:"type"`
	Timestamp  uint64 `json:"timestamp"`
	Visibility string `json:"visibility"`
	Mimetype   string `json:"mimetype"`
	Basename   string `json:"basename"`
	Dirname    string `json:"dirname"`
}

type treeItemResponse struct {
	Path       string                `json:"path"`
	Size       uint64                `json:"size"`
	Type       string                `json:"type"`
	Timestamp  uint64                `json:"timestamp"`
	Visibility string                `json:"visibility"`
	Mimetype   string                `json:"mimetype"`
	Basename   string                `json:"basename"`
	Dirname    string                `json:"dirname"`
	Props      treeItemPropsResponse `json:"props"`
}

type treeItemPropsResponse struct {
	HasSubdirectories bool `json:"hasSubdirectories"`
}

func newCreateDirectoryResponse(fileInfo *daemon.FileDetails, relativePath string) createDirectoryResponse {
	visibility := calculateVisibility(fileInfo.Perm)
	dirname := filepath.Dir(relativePath)
	if dirname == "." {
		dirname = ""
	}

	directoryItem := directoryItemResponse{
		Path:       relativePath,
		Size:       fileInfo.Size,
		Type:       "dir",
		Timestamp:  fileInfo.ModificationTime,
		Visibility: visibility,
		Mimetype:   "",
		Basename:   fileInfo.Name,
		Dirname:    dirname,
	}

	treeItem := treeItemResponse{
		Path:       relativePath,
		Size:       fileInfo.Size,
		Type:       "dir",
		Timestamp:  fileInfo.ModificationTime,
		Visibility: visibility,
		Mimetype:   "",
		Basename:   fileInfo.Name,
		Dirname:    dirname,
		Props: treeItemPropsResponse{
			HasSubdirectories: false,
		},
	}

	return createDirectoryResponse{
		Result: resultResponse{
			Status:  "success",
			Message: "Directory created!",
		},
		Directory: directoryItem,
		Tree:      []treeItemResponse{treeItem},
	}
}

func calculateVisibility(perm uint32) string {
	const worldReadable = 0o004

	if perm&worldReadable != 0 {
		return "public"
	}

	return "private"
}
