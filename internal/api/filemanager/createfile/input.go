package createfile

import "github.com/pkg/errors"

type createFileRequest struct {
	Disk string `json:"disk"`
	Path string `json:"path"`
	Name string `json:"name"`
}

func (in *createFileRequest) Validate() error {
	if in.Disk != "server" {
		return errors.Errorf("unsupported disk: %s, only 'server' disk is supported", in.Disk)
	}

	if in.Name == "" {
		return errors.New("name is required")
	}

	if in.Path == "" {
		in.Path = "."
	}

	return nil
}
