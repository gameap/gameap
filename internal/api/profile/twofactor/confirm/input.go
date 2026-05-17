package confirm

import (
	"strings"

	"github.com/gameap/gameap/pkg/api"
)

var ErrCodeRequired = api.NewValidationError("code field is required")

type confirmInput struct {
	Code string `json:"code"`
}

func (i *confirmInput) Validate() error {
	i.Code = strings.TrimSpace(i.Code)
	if i.Code == "" {
		return ErrCodeRequired
	}

	return nil
}
