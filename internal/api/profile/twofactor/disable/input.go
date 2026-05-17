package disable

import (
	"strings"

	"github.com/gameap/gameap/pkg/api"
)

var (
	ErrPasswordRequired = api.NewValidationError("password field is required")
	ErrCodeRequired     = api.NewValidationError("code field is required")
)

type disableInput struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

func (i *disableInput) Validate() error {
	i.Code = strings.TrimSpace(i.Code)

	if i.Password == "" {
		return ErrPasswordRequired
	}

	if i.Code == "" {
		return ErrCodeRequired
	}

	return nil
}
