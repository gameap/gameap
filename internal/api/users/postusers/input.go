package postusers

import (
	"fmt"
	"strings"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/validation"
	"github.com/pkg/errors"
)

const (
	maxLoginLength    = 255
	maxEmailLength    = 255
	maxNameLength     = 255
	minPasswordLength = 8
	maxPasswordLength = 64
)

var (
	ErrLoginRequired    = api.NewValidationError("login is required")
	ErrEmailRequired    = api.NewValidationError("email is required")
	ErrPasswordRequired = api.NewValidationError("password is required")
	ErrLoginTooLong     = api.NewValidationError(
		fmt.Sprintf("login must not exceed %d characters", maxLoginLength),
	)
	ErrEmailTooLong = api.NewValidationError(
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
	ErrLoginEmpty   = api.NewValidationError("login cannot be empty")
	ErrEmailEmpty   = api.NewValidationError("email cannot be empty")
	ErrNameEmpty    = api.NewValidationError("name cannot be empty")
)

type createUserInput struct {
	Login    string   `json:"login"`
	Email    string   `json:"email"`
	Password string   `json:"password"`
	Name     *string  `json:"name,omitempty"`
	Roles    []string `json:"roles"`
	Servers  []uint   `json:"servers"`
}

func (input *createUserInput) Validate() error {
	if input.Login == "" {
		return ErrLoginRequired
	}

	loginValue := strings.TrimSpace(input.Login)
	if loginValue == "" {
		return ErrLoginEmpty
	}

	if len(loginValue) > maxLoginLength {
		return ErrLoginTooLong
	}

	input.Login = loginValue

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

	if input.Password == "" {
		return ErrPasswordRequired
	}

	if len(input.Password) < minPasswordLength {
		return ErrPasswordTooShort
	}

	if len(input.Password) > maxPasswordLength {
		return ErrPasswordTooLong
	}

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

	return nil
}

func (input *createUserInput) ToDomain() (*domain.User, error) {
	hashedPassword, err := auth.HashPassword(strings.TrimSpace(input.Password))
	if err != nil {
		return nil, errors.WithMessage(err, "failed to hash password")
	}

	var name *string
	if input.Name != nil {
		nameValue := strings.TrimSpace(*input.Name)
		name = &nameValue
	}

	return &domain.User{
		Login:    strings.TrimSpace(input.Login),
		Email:    strings.TrimSpace(input.Email),
		Password: hashedPassword,
		Name:     name,
	}, nil
}
