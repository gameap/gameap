package postcommand

import (
	"strings"

	"github.com/pkg/errors"
)

type commandRequest struct {
	Command string `json:"command"`
}

func (c *commandRequest) Validate() error {
	if c.Command == "" {
		return errors.New("command is required")
	}

	if len(c.Command) > 127 {
		return errors.New("command must not exceed 127 characters")
	}

	c.Command = strings.TrimSpace(c.Command)

	return nil
}
