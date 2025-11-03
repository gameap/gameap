package postconsole

import (
	"github.com/pkg/errors"
)

type consoleInput struct {
	Command string `json:"command"`
}

func (in *consoleInput) validate() error {
	if in.Command == "" {
		return errors.New("command is required")
	}

	return nil
}
