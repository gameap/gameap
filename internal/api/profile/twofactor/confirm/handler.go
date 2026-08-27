package confirm

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
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
	audit     audit.Logger
}

func NewHandler(
	userRepo repositories.UserRepository,
	twoFactor twoFactorManager,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		userRepo:  userRepo,
		twoFactor: twoFactor,
		responder: responder,
		audit:     auditLogger,
	}
}

// ServeHTTP activates 2FA once the user proves the authenticator was
// provisioned with the pending secret. The recovery codes are generated and
// returned exactly once here.
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

	// A second factor may only be moved by someone holding a real session. See
	// base.EnsureSecondFactorChangeAllowedForSession for why a token must not.
	if err := base.EnsureSecondFactorChangeAllowedForSession(ctx); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	input := &confirmInput{}
	if err := json.NewDecoder(r.Body).Decode(input); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	if err := input.Validate(); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	user, ok := h.loadPendingEnrollment(ctx, rw, session.Login)
	if !ok {
		return
	}

	valid, usedStep, err := h.twoFactor.ValidateTOTP(*user.TwoFactorSecret, input.Code, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to validate code"))

		return
	}

	if !valid {
		h.responder.WriteError(ctx, rw, api.NewValidationError("invalid verification code"))

		return
	}

	plainCodes, encodedCodes, err := h.twoFactor.GenerateRecoveryCodes()
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate recovery codes"))

		return
	}

	step := usedStep
	user.TwoFactorEnabled = true
	user.TwoFactorLastUsedStep = &step
	user.TwoFactorRecoveryCodes = &encodedCodes

	if err = h.userRepo.Save(ctx, user); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to enable two-factor authentication"))

		return
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventTwoFactorEnabled,
		audit.CategoryAuthentication, "two_factor", "", "enable")

	h.responder.Write(ctx, rw, newRecoveryCodesResponse(plainCodes))
}

// loadPendingEnrollment returns the caller's account only when it is in the one
// state this endpoint can act on: existing, not already enrolled, and carrying
// the pending secret that /api/profile/2fa/setup stored.
func (h *Handler) loadPendingEnrollment(
	ctx context.Context, rw http.ResponseWriter, login string,
) (*domain.User, bool) {
	users, err := h.userRepo.Find(
		ctx, filters.FindUserByLogins(login), nil, &filters.Pagination{Limit: 1},
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find user"))

		return nil, false
	}

	if len(users) == 0 {
		h.responder.WriteError(ctx, rw, api.NewNotFoundError("user not found"))

		return nil, false
	}

	user := &users[0]

	if user.TwoFactorEnabled {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("two-factor authentication is already enabled"),
			http.StatusConflict,
		))

		return nil, false
	}

	if user.TwoFactorSecret == nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("no pending two-factor enrollment; call setup first"),
			http.StatusConflict,
		))

		return nil, false
	}

	return user, true
}
