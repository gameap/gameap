package recoverycodes

import (
	"encoding/json"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
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

// ServeHTTP regenerates the recovery codes, invalidating every previously
// issued one. The password is required so a hijacked session cannot silently
// rotate the codes and lock the legitimate owner out of their fallback.
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

	input := &recoveryCodesInput{}
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

	if vErr := auth.VerifyPassword(user.Password, input.Password); vErr != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("invalid credentials"),
			http.StatusUnauthorized,
		))

		return
	}

	plainCodes, encodedCodes, err := h.twoFactor.GenerateRecoveryCodes()
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate recovery codes"))

		return
	}

	user.TwoFactorRecoveryCodes = &encodedCodes

	if err = h.userRepo.Save(ctx, user); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to store recovery codes"))

		return
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventTwoFactorRecoveryRegenerate,
		audit.CategoryAuthentication, "two_factor", "", "recovery_regenerate")

	h.responder.Write(ctx, rw, newRecoveryCodesResponse(plainCodes))
}
