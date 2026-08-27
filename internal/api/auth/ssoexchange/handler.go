package ssoexchange

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/api/auth/login"
	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/mfanudge"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

// errInvalidTicket is the single answer to every rejection: expired, already
// used, never existed, wrong shape, bound to another address. Distinguishing
// them would tell an attacker which guesses are worth repeating.
var errInvalidTicket = errors.New("invalid or expired ticket")

// Handler exchanges a single-use SSO ticket for a real session.
//
// It is a guest endpoint by necessity — the whole point is that the browser
// arrives with no session — so it is wrapped in the login rate limiter and
// every ticket is consumed before it is validated, which makes a replay lose
// the race deterministically rather than depending on how fast the loser is.
type Handler struct {
	authService auth.Service
	userRepo    repositories.UserRepository
	rbac        adminChecker
	cache       tokenCache
	responder   base.Responder
	audit       audit.Logger

	// nudge and enrollmentTokenTTL make this path answer to the same admin-MFA
	// policy the password login does; a nil service disables the nudge, as it
	// does there.
	nudge              *mfanudge.Service
	enrollmentTokenTTL time.Duration

	// clientIPHeader is the operator-trusted proxy header, used to compare the
	// redeeming address against the one the ticket was bound to.
	clientIPHeader string
}

func NewHandler(
	authService auth.Service,
	userRepo repositories.UserRepository,
	rbac adminChecker,
	c tokenCache,
	clientIPHeader string,
	responder base.Responder,
	auditLogger audit.Logger,
	nudge *mfanudge.Service,
	enrollmentTokenTTL time.Duration,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		authService:        authService,
		userRepo:           userRepo,
		rbac:               rbac,
		cache:              c,
		responder:          responder,
		audit:              auditLogger,
		nudge:              nudge,
		enrollmentTokenTTL: enrollmentTokenTTL,
		clientIPHeader:     clientIPHeader,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	input := &exchangeInput{}

	if err := json.NewDecoder(r.Body).Decode(input); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request"),
			http.StatusBadRequest,
		))

		return
	}

	if err := input.Validate(); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	payload, ok := h.consumeTicket(rw, r, input.Ticket)
	if !ok {
		return
	}

	user, ok := h.loadUser(ctx, rw, payload.UserID)
	if !ok {
		return
	}

	// Repeated from the minting side: the account may have been promoted in
	// between, and a ticket must never grant an administrative session to
	// anyone but the account that minted it.
	isAdmin, err := h.rbac.Can(ctx, user.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check permissions"))

		return
	}

	// Compared against the recorded issuer rather than trusting that minting once
	// succeeded: a regular user promoted between minting and redeeming carries a
	// ticket whose issuer is somebody else, and is refused here. A payload with
	// no issuer fails the same way — a loaded user's id is never zero — so an
	// unreadable or hand-made payload fails closed.
	if isAdmin && payload.IssuerID != user.ID {
		h.reject(ctx, rw, "sso_target_is_other_admin")

		return
	}

	if user.TwoFactorEnabled {
		// The ticket buys a challenge and nothing more. That challenge is
		// finished at /api/auth/2fa/verify, which does not repeat these checks:
		// the cached challenge records only the user, not where it came from. It
		// does not have to — a challenge exists only because a password or a
		// validated ticket already produced one, so this is the last checkpoint.
		h.issueTwoFactorChallenge(ctx, rw, user, payload)

		return
	}

	// An administrator without a second factor is not turned away: a customer who
	// administers their own panel would have no way in at all. Instead the same
	// admin-MFA policy the password login applies decides how far this session
	// goes — a full one while the grace window lasts, carrying the nudge that
	// asks for enrolment, and an enrollment-scoped one once it closes.
	nudge := login.EvaluateMFANudge(ctx, h.nudge, h.userRepo, user, isAdmin)

	if nudge != nil && nudge.HardFail {
		h.issueMFAEnrollmentSession(ctx, rw, user, payload, nudge)

		return
	}

	token, err := h.authService.GenerateTokenForUser(user, login.DefaultTokenDuration)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate token"))

		return
	}

	h.recordRedemption(ctx, user, payload, "sso_ticket_redeem")

	response := newExchangeResponse(user, token, login.DefaultTokenDuration, payload.RedirectTo)
	response.MFANudge = nudge

	h.responder.Write(ctx, rw, response)
}

// issueMFAEnrollmentSession mirrors the password login's hard-fail branch: the
// administrator crossed the grace window without enrolling, so the ticket buys a
// session scoped to the 2FA-enrollment endpoints and nothing else.
func (h *Handler) issueMFAEnrollmentSession(
	ctx context.Context,
	rw http.ResponseWriter,
	user *domain.User,
	payload auth.SSOTicketPayload,
	nudge *mfanudge.View,
) {
	token, err := h.authService.GenerateMFAEnrollmentToken(user, h.enrollmentTokenTTL)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate enrollment token"))

		return
	}

	h.recordRedemption(ctx, user, payload, "sso_ticket_redeem_enrollment")

	response := newExchangeResponse(user, token, h.enrollmentTokenTTL, payload.RedirectTo)
	response.MFAEnrollmentRequired = true
	response.MFANudge = nudge

	h.responder.Write(ctx, rw, response)
}

// recordRedemption writes the pair of events every successful redemption leaves:
// the login itself, and the SSO-specific one naming who granted it.
func (h *Handler) recordRedemption(
	ctx context.Context, user *domain.User, payload auth.SSOTicketPayload, action string,
) {
	audit.LoginSuccess(ctx, h.audit, user.ID, user.Login)
	audit.SensitiveOp(
		ctx, h.audit,
		audit.EventSSOTicketRedeem, audit.CategoryAuthentication,
		"user", strconv.FormatUint(uint64(user.ID), 10), action,
		slog.Uint64("issuer_id", uint64(payload.IssuerID)),
	)
}

// consumeTicket validates the shape, then atomically pulls the entry (read and
// delete in one step) before looking at its contents. The atomic pull is what
// makes a replay lose the race deterministically: a ticket that fails any check
// below is already gone, and two redemptions cannot both read it.
func (h *Handler) consumeTicket(
	rw http.ResponseWriter, r *http.Request, ticket string,
) (auth.SSOTicketPayload, bool) {
	ctx := r.Context()

	// Reject anything not shaped like a ticket before it reaches the cache, so
	// a session token or PAT can never be fed in here.
	if !auth.IsSSOTicket(ticket) {
		h.reject(ctx, rw, "sso_ticket_malformed")

		return auth.SSOTicketPayload{}, false
	}

	key := auth.SSOTicketCacheKey(ticket)

	raw, err := h.cache.Pull(ctx, key)
	if err != nil {
		if errors.Is(err, cache.ErrNotFound) {
			h.reject(ctx, rw, "sso_ticket_not_found")

			return auth.SSOTicketPayload{}, false
		}

		// The secret has been on the wire but we could not consume it, so the
		// entry may still be live until its TTL. Record it: this is the only
		// signal that a redeemable ticket outlived its single-use guarantee.
		audit.TokenRejected(ctx, h.audit, "sso_ticket_consume_error")
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to consume ticket"))

		return auth.SSOTicketPayload{}, false
	}

	payload, err := auth.UnmarshalSSOTicketPayload(raw)
	if err != nil {
		h.reject(ctx, rw, "sso_ticket_corrupt")

		return auth.SSOTicketPayload{}, false
	}

	// A payload without a positive deadline is invalid, not eternal. This
	// check is the backstop for a cache backend that does not enforce its own
	// TTL, so a missing or non-positive ExpiresAt must fail closed.
	if payload.ExpiresAt <= 0 || time.Now().Unix() > payload.ExpiresAt {
		h.reject(ctx, rw, "sso_ticket_expired")

		return auth.SSOTicketPayload{}, false
	}

	if payload.ClientIP != "" && payload.ClientIP != audit.ClientIP(r, h.clientIPHeader) {
		h.reject(ctx, rw, "sso_ticket_ip_mismatch")

		return auth.SSOTicketPayload{}, false
	}

	return payload, true
}

func (h *Handler) loadUser(
	ctx context.Context, rw http.ResponseWriter, userID uint,
) (*domain.User, bool) {
	users, err := h.userRepo.Find(ctx, filters.FindUserByIDs(userID), nil, &filters.Pagination{Limit: 1})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to load user"))

		return nil, false
	}

	if len(users) == 0 {
		h.reject(ctx, rw, "sso_user_missing")

		return nil, false
	}

	return &users[0], true
}

// issueTwoFactorChallenge hands back the same challenge the password login
// would produce. The ticket is already consumed at this point, so the customer
// gets one shot at the second factor per ticket.
func (h *Handler) issueTwoFactorChallenge(
	ctx context.Context,
	rw http.ResponseWriter,
	user *domain.User,
	payload auth.SSOTicketPayload,
) {
	// SSO tickets never carry a "remember me": the customer arrives through a
	// one-shot link rather than a login form, so the eventual session takes the
	// default lifetime. The shared helper also records the TwoFactorChallengeIssued
	// audit event this path used to omit.
	challengeToken, err := login.IssueTwoFactorChallenge(ctx, h.cache, h.audit, user, false)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	audit.SensitiveOp(
		ctx, h.audit,
		audit.EventSSOTicketRedeem, audit.CategoryAuthentication,
		"user", strconv.FormatUint(uint64(user.ID), 10), "sso_ticket_redeem_2fa",
		slog.Uint64("issuer_id", uint64(payload.IssuerID)),
	)

	h.responder.Write(ctx, rw, twoFactorChallengeResponse{
		TwoFactorRequired: true,
		ChallengeToken:    challengeToken,
		ExpiresIn:         int64(login.ChallengeTokenDuration.Seconds()),
		RedirectTo:        payload.RedirectTo,
	})
}

func (h *Handler) reject(ctx context.Context, rw http.ResponseWriter, reason string) {
	audit.TokenRejected(ctx, h.audit, reason)
	h.responder.WriteError(ctx, rw, api.WrapHTTPError(errInvalidTicket, http.StatusUnauthorized))
}
