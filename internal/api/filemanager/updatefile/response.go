package updatefile

import (
	"path/filepath"

	"github.com/gameap/gameap/internal/daemon"
)

type updateFileResponse struct {
	Result resultResponse   `json:"result"`
	File   fileItemResponse `json:"file"`
}

type resultResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type fileItemResponse struct {
	Path       string `json:"path"`
	Size       uint64 `json:"size"`
	Type       string `json:"type"`
	Timestamp  uint64 `json:"timestamp"`
	Visibility string `json:"visibility"`
	Mimetype   string `json:"mimetype"`
	Basename   string `json:"basename"`
	Dirname    string `json:"dirname"`
	Extension  string `json:"extension"`
	Filename   string `json:"filename"`
}

func newUpdateFileResponse(fileInfo *daemon.FileDetails, relativePath string) updateFileResponse {
	visibility := calculateVisibility(fileInfo.Perm)
	dirname := filepath.Dir(relativePath)
	if dirname == "." {
		dirname = ""
	}

	extension := filepath.Ext(fileInfo.Name)
	filename := fileInfo.Name
	if extension != "" {
		filename = filename[:len(filename)-len(extension)]
		extension = extension[1:]
	}

	fileItem := fileItemResponse{
		Path:       relativePath,
		Size:       fileInfo.Size,
		Type:       "file",
		Timestamp:  fileInfo.ModificationTime,
		Visibility: visibility,
		Mimetype:   fileInfo.Mime,
		Basename:   fileInfo.Name,
		Dirname:    dirname,
		Extension:  extension,
		Filename:   filename,
	}

	return updateFileResponse{
		Result: resultResponse{
			Status:  "success",
			Message: "File updated!",
		},
		File: fileItem,
	}
}

func calculateVisibility(perm uint32) string {
	const worldReadable = 0o004

	if perm&worldReadable != 0 {
		return "public"
	}

	return "private"
}
