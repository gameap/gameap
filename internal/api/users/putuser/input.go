package putuser

import (
	"fmt"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/flexible"
	"github.com/gameap/gameap/pkg/validation"
	"github.com/pkg/errors"
)

const (
	maxEmailLength = 255
	maxNameLength  = 255
)

var (
	ErrEmailRequired = api.NewValidationError("email is required")
	ErrEmailTooLong  = api.NewValidationError(
		fmt.Sprintf("email must not exceed %d characters", maxEmailLength),
	)
	ErrNameTooLong = api.NewValidationError(
		fmt.Sprintf("name must not exceed %d characters", maxNameLength),
	)
	ErrInvalidEmail = api.NewValidationError("email is not valid")
	ErrEmailEmpty   = api.NewValidationError("email cannot be empty")
	ErrNameEmpty    = api.NewValidationError("name cannot be empty")
)

type updateUserInput struct {
	Email    string          `json:"email"`
	Name     *string         `json:"name,omitempty"`
	Password *string         `json:"password,omitempty"`
	Roles    []string        `json:"roles"`
	Servers  []flexible.Uint `json:"servers"`
}

func (input *updateUserInput) Validate() error {
	if input.Email == "" {
		return ErrEmailRequired
	}

	emailValue := strings.TrimSpace(input.Email)
	if emailValue == "" {
		return ErrEmailEmpty
	}

	if len(emailValue) > maxEmailLength {
		return ErrEmailTooLong
	}

	if !validation.IsEmail(emailValue) {
		return ErrInvalidEmail
	}

	input.Email = emailValue

	if input.Name != nil {
		nameValue := strings.TrimSpace(*input.Name)
		if nameValue == "" {
			return ErrNameEmpty
		}

		if len(nameValue) > maxNameLength {
			return ErrNameTooLong
		}

		input.Name = &nameValue
	}

	// Password is optional on admin update: an empty value means "no change".
	// When supplied, it must satisfy the ASVS §2.1.1 / §2.1.2 policy.
	if input.Password != nil && *input.Password != "" {
		if err := auth.ValidatePassword(*input.Password); err != nil {
			return err
		}
	}

	return nil
}

func (input *updateUserInput) Apply(user *domain.User) error {
	user.Email = input.Email

	if input.Name != nil {
		user.Name = input.Name
	}

	if input.Password != nil && *input.Password != "" {
		hashedPassword, err := auth.HashPassword(*input.Password)
		if err != nil {
			return errors.WithMessage(err, "failed to hash password")
		}

		user.Password = hashedPassword
		user.SetPasswordChangedAt(new(time.Now()))
	}

	return nil
}

// changedFields names what the request changes on the user, for the plugin
// event: field names only, never values. Roles and servers are always
// replaced by this endpoint, so they are listed whenever the request sends
// them.
func (input *updateUserInput) changedFields(user *domain.User) []string {
	fields := make([]string, 0, 5)

	if user.Email != input.Email {
		fields = append(fields, "email")
	}

	if input.Name != nil && (user.Name == nil || *user.Name != *input.Name) {
		fields = append(fields, "name")
	}

	if input.Password != nil && *input.Password != "" {
		fields = append(fields, "password")
	}

	if input.Roles != nil {
		fields = append(fields, "roles")
	}

	if input.Servers != nil {
		fields = append(fields, "servers")
	}

	return fields
}

func (input *updateUserInput) ServerIDs() []uint {
	result := make([]uint, 0, len(input.Servers))
	for _, s := range input.Servers {
		result = append(result, s.Uint())
	}

	return result
}
