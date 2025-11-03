package failservertask

import (
	"github.com/pkg/errors"
)

type failServerTaskInput struct {
	Output string `json:"output"`
}

func (in *failServerTaskInput) Validate() error {
	if in.Output == "" {
		return errors.New("output is required")
	}

	return nil
}
