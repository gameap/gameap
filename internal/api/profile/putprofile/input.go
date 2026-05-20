package putprofile

import (
	"fmt"
	"strings"

	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
)

const (
	maxNameLength = 255
)

var (
	ErrNameTooLong = api.NewValidationError(
		fmt.Sprintf("name must not exceed %d characters", maxNameLength),
	)
	ErrNameEmpty            = api.NewValidationError("name cannot be empty")
	ErrCurrentPasswordEmpty = api.NewValidationError("current password cannot be empty")
)

type updateProfileInput struct {
	Name            *string `json:"name,omitempty"`             // maxlen=255
	Password        *string `json:"password,omitempty"`         // OWASP ASVS 4.0.3 §2.1.1 / §2.1.2
	CurrentPassword *string `json:"current_password,omitempty"` //
}

func (u *updateProfileInput) Validate() error {
	if u.Name != nil {
		nameValue := strings.TrimSpace(*u.Name)
		if nameValue == "" {
			return ErrNameEmpty
		}

		if len(nameValue) > maxNameLength {
			return ErrNameTooLong
		}

		u.Name = &nameValue
	}

	if u.Password != nil {
		if err := auth.ValidatePassword(*u.Password); err != nil {
			return err
		}
	}

	if u.CurrentPassword != nil {
		currentPasswordValue := *u.CurrentPassword
		if currentPasswordValue == "" {
			return ErrCurrentPasswordEmpty
		}
	}

	return nil
}
