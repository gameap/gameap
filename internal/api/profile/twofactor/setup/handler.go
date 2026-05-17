package setup

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	userRepo  repositories.UserRepository
	twoFactor twoFactorManager
	responder base.Responder
}

func NewHandler(
	userRepo repositories.UserRepository,
	twoFactor twoFactorManager,
	responder base.Responder,
) *Handler {
	return &Handler{
		userRepo:  userRepo,
		twoFactor: twoFactor,
		responder: responder,
	}
}

// ServeHTTP starts (or restarts, while still unconfirmed) TOTP enrollment. The
// secret is stored encrypted immediately but TwoFactorEnabled stays false, so
// the pending secret is inert until /api/profile/2fa/confirm proves the user
// provisioned their authenticator.
func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := auth.SessionFromContext(ctx)
	if !session.IsAuthenticated() {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("user not authenticated"),
			http.StatusUnauthorized,
		))

		return
	}

	users, err := h.userRepo.Find(
		ctx, filters.FindUserByLogins(session.Login), nil, &filters.Pagination{Limit: 1},
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find user"))

		return
	}

	if len(users) == 0 {
		h.responder.WriteError(ctx, rw, api.NewNotFoundError("user not found"))

		return
	}

	user := &users[0]

	if user.TwoFactorEnabled {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("two-factor authentication is already enabled"),
			http.StatusConflict,
		))

		return
	}

	secret, otpauthURI, err := h.twoFactor.GenerateSecret(user.Login)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate two-factor secret"))

		return
	}

	encrypted, err := h.twoFactor.EncryptSecret(secret)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to encrypt two-factor secret"))

		return
	}

	user.TwoFactorSecret = &encrypted
	user.TwoFactorRecoveryCodes = nil
	user.TwoFactorLastUsedStep = nil

	if err = h.userRepo.Save(ctx, user); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to store pending two-factor secret"))

		return
	}

	h.responder.Write(ctx, rw, newSetupResponse(secret, otpauthURI))
}
