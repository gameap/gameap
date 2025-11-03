package login

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

const (
	DefaultTokenDuration = 24 * time.Hour
	RememberMeDuration   = 30 * 24 * time.Hour // 30 days
)

type Handler struct {
	userRepo    repositories.UserRepository
	responder   base.Responder
	authService auth.Service
}

func NewHandler(
	authService auth.Service, userRepo repositories.UserRepository, responder base.Responder,
) *Handler {
	return &Handler{
		userRepo:    userRepo,
		responder:   responder,
		authService: authService,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	input := &loginInput{}

	err := json.NewDecoder(r.Body).Decode(input)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	err = input.Validate()
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	// Find user by email or login
	var users []domain.User
	var filter *filters.FindUser

	if input.IsEmailLogin() {
		filter = &filters.FindUser{Emails: []string{input.Email}}
	} else {
		filter = &filters.FindUser{Logins: []string{input.Login}}
	}

	users, err = h.userRepo.Find(ctx, filter, nil, &filters.Pagination{
		Limit:  1,
		Offset: 0,
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find user"))

		return
	}

	if len(users) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("invalid credentials"),
			http.StatusUnauthorized,
		))

		return
	}

	user := users[0]

	// Verify password
	err = auth.VerifyPassword(user.Password, input.Password)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("invalid credentials"),
			http.StatusUnauthorized,
		))

		return
	}

	duration := DefaultTokenDuration
	if input.RememberMe() {
		duration = RememberMeDuration
	}

	// Generate JWT token
	token, err := h.authService.GenerateTokenForUser(&user, duration)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate token"))

		return
	}

	response := newLoginResponseFromUser(&user, token, DefaultTokenDuration)
	h.responder.Write(ctx, rw, response)
}
