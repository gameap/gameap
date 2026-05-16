package shorttoken

import (
	"net/http"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
)

const (
	secretLength = 48
	maxTokenTTL  = 10 * time.Second
)

// Handler issues a single-use, short-lived authorization token for the
// authenticated caller. The token is meant for contexts where a credential
// must travel in the URL (WebSocket upgrade, file download) — it expires
// within seconds and is invalidated on first use by the auth middleware, so a
// copy captured from logs/history/proxies is worthless after the connection
// is established.
//
// The minting request itself must authenticate with the long-lived token in
// the Authorization header (never the URL); the route is wired without guest
// access and rejects short-lived tokens, so it cannot be chained.
type Handler struct {
	cache     tokenCache
	ttl       time.Duration
	responder base.Responder
	audit     audit.Logger
}

func NewHandler(
	c tokenCache,
	ttl time.Duration,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}
	if ttl <= 0 || ttl > maxTokenTTL {
		ttl = maxTokenTTL
	}

	return &Handler{
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

	secret, err := pkgstrings.CryptoRandomString(secretLength)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to generate token"),
			http.StatusInternalServerError,
		))

		return
	}

	token := auth.ShortLivedTokenPrefix + secret

	payload := auth.ShortLivedPayload{
		UserID: session.User.ID,
		Login:  session.User.Login,
		Email:  session.User.Email,
	}
	if session.IsTokenSession() {
		payload.PATID = session.Token.ID
		if session.Token.Abilities != nil {
			payload.Abilities = *session.Token.Abilities
		}
	}

	encoded, err := auth.MarshalShortLivedPayload(payload)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to encode token payload"))

		return
	}

	err = h.cache.Set(ctx, auth.ShortLivedCacheKey(token), encoded, cache.WithExpiration(h.ttl))
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to store token"),
			http.StatusInternalServerError,
		))

		return
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventShortLivedTokenIssue, audit.CategoryTokenOp,
		"token", "", "issue")

	h.responder.Write(ctx, rw, newTokenResponse(token, int64(h.ttl.Seconds())))
}
