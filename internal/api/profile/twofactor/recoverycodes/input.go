package recoverycodes

import "github.com/gameap/gameap/pkg/api"

var ErrPasswordRequired = api.NewValidationError("password field is required")

type recoveryCodesInput struct {
	Password string `json:"password"`
}

func (i *recoveryCodesInput) Validate() error {
	if i.Password == "" {
		return ErrPasswordRequired
	}

	return nil
}
