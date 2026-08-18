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
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/gameap/gameap/pkg/twofactor"
	"github.com/pkg/errors"
)

const challengeSecretLength = 48

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
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		authService:    authService,
		userRepo:       userRepo,
		rbac:           rbac,
		cache:          c,
		responder:      responder,
		audit:          auditLogger,
		clientIPHeader: clientIPHeader,
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
	// between, and a ticket must never grant an administrative session.
	isAdmin, err := h.rbac.Can(ctx, user.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check permissions"))

		return
	}

	if isAdmin {
		h.reject(ctx, rw, "sso_target_is_admin")

		return
	}

	if user.TwoFactorEnabled {
		h.issueTwoFactorChallenge(ctx, rw, user, payload)

		return
	}

	token, err := h.authService.GenerateTokenForUser(user, login.DefaultTokenDuration)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate token"))

		return
	}

	audit.LoginSuccess(ctx, h.audit, user.ID, user.Login)
	audit.SensitiveOp(
		ctx, h.audit,
		audit.EventSSOTicketRedeem, audit.CategoryAuthentication,
		"user", strconv.FormatUint(uint64(user.ID), 10), "sso_ticket_redeem",
		slog.Uint64("issuer_id", uint64(payload.IssuerID)),
	)

	h.responder.Write(ctx, rw, newExchangeResponse(
		user, token, login.DefaultTokenDuration, payload.RedirectTo,
	))
}

// consumeTicket validates the shape, reads the entry and deletes it before
// looking at its contents. Deleting first is what makes a replay lose the race
// deterministically: a ticket that fails any check below is already gone.
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

	raw, err := h.cache.Get(ctx, key)
	if err != nil {
		if errors.Is(err, cache.ErrNotFound) {
			h.reject(ctx, rw, "sso_ticket_not_found")

			return auth.SSOTicketPayload{}, false
		}

		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to load ticket"))

		return auth.SSOTicketPayload{}, false
	}

	if delErr := h.cache.Delete(ctx, key); delErr != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(delErr, "failed to consume ticket"))

		return auth.SSOTicketPayload{}, false
	}

	payload, err := auth.UnmarshalSSOTicketPayload(raw)
	if err != nil {
		h.reject(ctx, rw, "sso_ticket_corrupt")

		return auth.SSOTicketPayload{}, false
	}

	if payload.ExpiresAt > 0 && time.Now().Unix() > payload.ExpiresAt {
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
	secret, err := pkgstrings.CryptoRandomString(challengeSecretLength)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to generate challenge token"),
			http.StatusInternalServerError,
		))

		return
	}

	challengeToken := twofactor.ChallengeTokenPrefix + secret

	encoded, err := twofactor.MarshalChallengePayload(twofactor.ChallengePayload{
		UserID:    user.ID,
		Login:     user.Login,
		Email:     user.Email,
		ExpiresAt: time.Now().Add(login.ChallengeTokenDuration).Unix(),
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to encode challenge"))

		return
	}

	err = h.cache.Set(
		ctx,
		twofactor.ChallengeCacheKey(challengeToken),
		encoded,
		cache.WithExpiration(login.ChallengeTokenDuration),
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to store challenge"))

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
