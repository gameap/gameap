package login

import (
	"github.com/gameap/gameap/pkg/api"
)

var (
	ErrLoginIsRequired    = api.NewValidationError("login or email fields are required")
	ErrPasswordIsRequired = api.NewValidationError("password field is required")
)

type loginInput struct {
	Login    string `json:"login"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Remember string `json:"remember"`
	// Captcha carries the provider-issued solution token. Required only
	// when a CAPTCHA provider is configured; the verifier enforces its
	// presence so the error message can be provider-aware.
	Captcha string `json:"captcha"`
}

func (l *loginInput) Validate() error {
	if l.Login == "" && l.Email == "" {
		return ErrLoginIsRequired
	}

	if l.Password == "" {
		return ErrPasswordIsRequired
	}

	return nil
}

func (l *loginInput) RememberMe() bool {
	return l.Remember == "on" || l.Remember == "true"
}

func (l *loginInput) IsEmailLogin() bool {
	return l.Email != ""
}
