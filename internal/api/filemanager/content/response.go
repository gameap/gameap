package content

import (
	"path/filepath"
	"strings"

	"github.com/gameap/gameap/internal/daemon"
)

type contentResponse struct {
	Result      resultResponse          `json:"result"`
	Directories []directoryItemResponse `json:"directories"`
	Files       []fileItemResponse      `json:"files"`
}

type resultResponse struct {
	Status  string  `json:"status"`
	Message *string `json:"message"`
}

type directoryItemResponse struct {
	Path      string `json:"path"`
	Timestamp uint64 `json:"timestamp"`
	Type      string `json:"type"`
	Dirname   string `json:"dirname"`
	Basename  string `json:"basename"`
}

type fileItemResponse struct {
	Path       string  `json:"path"`
	Timestamp  uint64  `json:"timestamp"`
	Type       string  `json:"type"`
	Visibility string  `json:"visibility"`
	Size       uint64  `json:"size"`
	Dirname    string  `json:"dirname"`
	Basename   string  `json:"basename"`
	Extension  *string `json:"extension,omitempty"`
	Filename   string  `json:"filename"`
}

func newContentResponse(fileInfoList []*daemon.FileInfo, directory string) contentResponse {
	directories := make([]directoryItemResponse, 0)
	files := make([]fileItemResponse, 0)

	for _, fileInfo := range fileInfoList {
		fullPath := filepath.Join(directory, fileInfo.Name)
		dirname := directory
		if dirname == "." {
			dirname = ""
		}

		switch fileInfo.Type {
		case daemon.FileTypeDir:
			directories = append(directories, directoryItemResponse{
				Path:      fullPath,
				Timestamp: fileInfo.TimeModified,
				Type:      "dir",
				Dirname:   dirname,
				Basename:  fileInfo.Name,
			})
		case daemon.FileTypeFile:
			filename, extension := parseFilename(fileInfo.Name)
			visibility := calculateVisibility(fileInfo.Perm)

			fileResponse := fileItemResponse{
				Path:       fullPath,
				Timestamp:  fileInfo.TimeModified,
				Type:       "file",
				Visibility: visibility,
				Size:       fileInfo.Size,
				Dirname:    dirname,
				Basename:   fileInfo.Name,
				Filename:   filename,
			}

			if extension != "" {
				fileResponse.Extension = &extension
			}

			files = append(files, fileResponse)
		}
	}

	return contentResponse{
		Result: resultResponse{
			Status:  "success",
			Message: nil,
		},
		Directories: directories,
		Files:       files,
	}
}

func parseFilename(name string) (filename string, extension string) {
	ext := filepath.Ext(name)
	if ext != "" {
		extension = strings.TrimPrefix(ext, ".")
		filename = strings.TrimSuffix(name, ext)
	} else {
		filename = name
		extension = ""
	}

	return filename, extension
}

func calculateVisibility(perm uint32) string {
	const worldReadable = 0o004

	if perm&worldReadable != 0 {
		return "public"
	}

	return "private"
}
