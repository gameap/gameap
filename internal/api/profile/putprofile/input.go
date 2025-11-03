package putprofile

import (
	"fmt"
	"strings"

	"github.com/gameap/gameap/pkg/api"
)

const (
	maxNameLength     = 255
	minPasswordLength = 8
	maxPasswordLength = 64
)

var (
	ErrNameTooLong = api.NewValidationError(
		fmt.Sprintf("name must not exceed %d characters", maxNameLength),
	)
	ErrPasswordTooShort = api.NewValidationError(
		fmt.Sprintf("password must be at least %d characters long", minPasswordLength),
	)
	ErrPasswordTooLong = api.NewValidationError(
		fmt.Sprintf("password must not exceed %d characters", maxPasswordLength),
	)
	ErrNameEmpty            = api.NewValidationError("name cannot be empty")
	ErrPasswordEmpty        = api.NewValidationError("password cannot be empty")
	ErrCurrentPasswordEmpty = api.NewValidationError("current password cannot be empty")
)

type updateProfileInput struct {
	Name            *string `json:"name,omitempty"`             // maxlen=255
	Password        *string `json:"password,omitempty"`         // minlen=8
	CurrentPassword *string `json:"current_password,omitempty"` //
}

func (u *updateProfileInput) Validate() error {
	// Validate name if provided
	if u.Name != nil {
		nameValue := strings.TrimSpace(*u.Name)
		if nameValue == "" {
			return ErrNameEmpty
		}

		if len(nameValue) > maxNameLength {
			return ErrNameTooLong
		}

		// Update the name value after trimming
		u.Name = &nameValue
	}

	// Validate password if provided
	if u.Password != nil {
		passwordValue := *u.Password
		if passwordValue == "" {
			return ErrPasswordEmpty
		}

		if len(passwordValue) < minPasswordLength {
			return ErrPasswordTooShort
		}

		if len(passwordValue) > maxPasswordLength {
			return ErrPasswordTooLong
		}
	}

	// Validate current password if provided
	if u.CurrentPassword != nil {
		currentPasswordValue := *u.CurrentPassword
		if currentPasswordValue == "" {
			return ErrCurrentPasswordEmpty
		}
	}

	return nil
}
