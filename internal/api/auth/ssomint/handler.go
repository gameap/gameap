package ssomint

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
)

const (
	ticketSecretLength = 48

	// maxTicketTTL caps whatever an operator configures. The ticket travels in
	// a URL, so its window has to stay short even if someone sets an hour.
	maxTicketTTL = 120 * time.Second
)

var (
	errTargetNotFound = api.NewNotFoundError("user not found")
	errTargetIsAdmin  = errors.New("cannot issue a login ticket for an administrator")
)

// Handler mints a single-use ticket that logs one specific user into the
// panel. It exists for external systems that already own the customer
// relationship — a billing panel offering a "open my game panel" button —
// which cannot use a personal access token for this: a PAT is always issued
// for its own owner, and so is the short-lived URL token.
//
// The ticket is not a credential anywhere else: its prefix is unknown to the
// auth middleware, so presenting it as a Bearer token fails with 401. Only
// the exchange endpoint understands it.
type Handler struct {
	userRepo  repositories.UserRepository
	rbac      adminChecker
	cache     tokenCache
	ttl       time.Duration
	responder base.Responder
	audit     audit.Logger
}

func NewHandler(
	userRepo repositories.UserRepository,
	rbac adminChecker,
	c tokenCache,
	ttl time.Duration,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	if ttl <= 0 || ttl > maxTicketTTL {
		ttl = maxTicketTTL
	}

	return &Handler{
		userRepo:  userRepo,
		rbac:      rbac,
		cache:     c,
		ttl:       ttl,
		responder: responder,
		audit:     auditLogger,
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

	input := &ticketInput{}

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

	user, ok := h.loadTarget(ctx, rw, input.UserID)
	if !ok {
		return
	}

	// Refusing administrators is what keeps this endpoint from turning a
	// scoped integration token into panel takeover: whoever steals the token
	// out of the billing database can log in as a customer, never as an admin.
	// The exchange endpoint repeats the check, closing the window between
	// minting and redeeming.
	isAdmin, err := h.rbac.Can(ctx, user.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check target permissions"))

		return
	}

	if isAdmin {
		audit.AccessDenied(ctx, h.audit, "user", idString(user.ID), "sso_target_is_admin")
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(errTargetIsAdmin, http.StatusForbidden))

		return
	}

	ticket, err := h.issue(ctx, session, user, input)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	audit.SensitiveOp(
		ctx, h.audit,
		audit.EventSSOTicketIssue, audit.CategoryTokenOp,
		"user", idString(user.ID), "sso_ticket_issue",
		slog.Bool("ip_bound", input.ClientIP != ""),
	)

	h.responder.Write(ctx, rw, ticketResponse{
		Ticket:     ticket,
		ExpiresIn:  int64(h.ttl.Seconds()),
		RedirectTo: input.RedirectTo,
	})
}

func (h *Handler) loadTarget(
	ctx context.Context, rw http.ResponseWriter, userID uint,
) (*domain.User, bool) {
	users, err := h.userRepo.Find(ctx, filters.FindUserByIDs(userID), nil, &filters.Pagination{Limit: 1})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find user"))

		return nil, false
	}

	if len(users) == 0 {
		h.responder.WriteError(ctx, rw, errTargetNotFound)

		return nil, false
	}

	return &users[0], true
}

// issue mints the secret and stores only its hash-derived key, so a cache dump
// never yields a usable ticket.
func (h *Handler) issue(
	ctx context.Context,
	session *auth.Session,
	user *domain.User,
	input *ticketInput,
) (string, error) {
	secret, err := pkgstrings.CryptoRandomString(ticketSecretLength)
	if err != nil {
		return "", api.WrapHTTPError(
			errors.WithMessage(err, "failed to generate ticket"),
			http.StatusInternalServerError,
		)
	}

	ticket := auth.SSOTicketPrefix + secret

	payload := auth.SSOTicketPayload{
		UserID:     user.ID,
		Login:      user.Login,
		IssuerID:   session.User.ID,
		RedirectTo: input.RedirectTo,
		ClientIP:   input.ClientIP,
		ExpiresAt:  time.Now().Add(h.ttl).Unix(),
	}

	if session.Token != nil {
		payload.IssuerPATID = session.Token.ID
	}

	encoded, err := auth.MarshalSSOTicketPayload(payload)
	if err != nil {
		return "", errors.WithMessage(err, "failed to encode ticket payload")
	}

	err = h.cache.Set(ctx, auth.SSOTicketCacheKey(ticket), encoded, cache.WithExpiration(h.ttl))
	if err != nil {
		return "", errors.WithMessage(err, "failed to store ticket")
	}

	return ticket, nil
}

func idString(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
