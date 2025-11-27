package putuser

import (
	"fmt"
	"strings"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/flexible"
	"github.com/gameap/gameap/pkg/validation"
	"github.com/pkg/errors"
)

const (
	maxEmailLength    = 255
	maxNameLength     = 255
	minPasswordLength = 8
	maxPasswordLength = 64
)

var (
	ErrEmailRequired = api.NewValidationError("email is required")
	ErrEmailTooLong  = api.NewValidationError(
		fmt.Sprintf("email must not exceed %d characters", maxEmailLength),
	)
	ErrNameTooLong = api.NewValidationError(
		fmt.Sprintf("name must not exceed %d characters", maxNameLength),
	)
	ErrPasswordTooShort = api.NewValidationError(
		fmt.Sprintf("password must be at least %d characters long", minPasswordLength),
	)
	ErrPasswordTooLong = api.NewValidationError(
		fmt.Sprintf("password must not exceed %d characters", maxPasswordLength),
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

	if input.Password != nil && *input.Password != "" {
		passwordValue := *input.Password
		if len(passwordValue) < minPasswordLength {
			return ErrPasswordTooShort
		}

		if len(passwordValue) > maxPasswordLength {
			return ErrPasswordTooLong
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
	}

	return nil
}

func (input *updateUserInput) ServerIDs() []uint {
	result := make([]uint, 0, len(input.Servers))
	for _, s := range input.Servers {
		result = append(result, s.Uint())
	}

	return result
}
