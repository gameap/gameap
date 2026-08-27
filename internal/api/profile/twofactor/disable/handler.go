package disable

import (
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

// ServeHTTP turns 2FA off. It deliberately requires both the password and a
// valid second factor: a hijacked session (no password) or a leaked password
// (no device) must not be enough to strip the protection on its own.
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

	input := &disableInput{}
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

	if !user.TwoFactorEnabled {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("two-factor authentication is not enabled"),
			http.StatusConflict,
		))

		return
	}

	needsRehash, vErr := auth.VerifyPassword(user.Password, input.Password)
	if vErr != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("invalid credentials"),
			http.StatusUnauthorized,
		))

		return
	}

	// Piggyback the pre-§2.1.2 → SHA-256+bcrypt hash upgrade on the Save that
	// already happens below for the 2FA state change. Best-effort: if rehashing
	// fails the upgrade simply retries on the next sensitive op.
	if needsRehash {
		if upgradedHash, hashErr := auth.HashPassword(input.Password); hashErr == nil {
			user.Password = upgradedHash
		}
	}

	if !h.codeIsValid(user, input.Code) {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("invalid verification code"),
			http.StatusUnauthorized,
		))

		return
	}

	user.TwoFactorEnabled = false
	user.TwoFactorSecret = nil
	user.TwoFactorRecoveryCodes = nil
	user.TwoFactorLastUsedStep = nil

	if err = h.userRepo.Save(ctx, user); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to disable two-factor authentication"))

		return
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventTwoFactorDisabled,
		audit.CategoryAuthentication, "two_factor", "", "disable")

	h.responder.Write(ctx, rw, newDisableResponse())
}

func (h *Handler) codeIsValid(user *domain.User, code string) bool {
	if user.TwoFactorSecret != nil {
		valid, _, err := h.twoFactor.ValidateTOTP(*user.TwoFactorSecret, code, user.TwoFactorLastUsedStep)
		if err == nil && valid {
			return true
		}
	}

	if user.TwoFactorRecoveryCodes != nil {
		_, used, err := h.twoFactor.ConsumeRecoveryCode(*user.TwoFactorRecoveryCodes, code)
		if err == nil && used {
			return true
		}
	}

	return false
}
