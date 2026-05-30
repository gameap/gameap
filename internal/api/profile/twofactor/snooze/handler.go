// Package snooze implements POST /api/profile/2fa/snooze: the authenticated
// user dismisses the admin-MFA nudge for the fixed snooze window
// (mfanudge.SnoozeDuration). The snooze deadline is stored in the user's
// metadata bag and the recomputed recommendation is returned so the frontend
// can hide the modal. A hard-failing user can never reach this endpoint — the
// enrollment scope guard does not opt it in.
package snooze

import (
	"net/http"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/mfanudge"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	userRepo  repositories.UserRepository
	nudge     *mfanudge.Service
	admin     adminChecker
	responder base.Responder
}

func NewHandler(
	userRepo repositories.UserRepository,
	nudge *mfanudge.Service,
	admin adminChecker,
	responder base.Responder,
) *Handler {
	return &Handler{
		userRepo:  userRepo,
		nudge:     nudge,
		admin:     admin,
		responder: responder,
	}
}

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

	user := session.User

	snoozedUntil := time.Now().Add(mfanudge.SnoozeDuration)
	user.SetMFASnoozedUntil(&snoozedUntil)

	if err := h.userRepo.Save(ctx, user); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to persist MFA snooze"))

		return
	}

	isAdmin, err := h.admin.Can(ctx, user.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check admin status"))

		return
	}

	rec := h.nudge.Compute(mfanudge.Snapshot{
		IsAdmin:          isAdmin,
		TwoFactorEnabled: user.TwoFactorEnabled,
		FirstShownAt:     user.MFAFirstShownAt(),
		SnoozedUntil:     user.MFASnoozedUntil(),
	})

	h.responder.Write(ctx, rw, snoozeResponse{MFANudge: mfanudge.NewView(rec)})
}

type snoozeResponse struct {
	MFANudge *mfanudge.View `json:"mfa_nudge"`
}
