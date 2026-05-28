package deletetoken

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

// patRevocationTTL bounds how long a revoked PAT identifier sits in the
// denylist. PATs do not carry an expiration claim — we conservatively keep
// the revocation entry for one year, which is longer than any realistic
// PAT lifetime and lets the cache backend prune the entry eventually.
const patRevocationTTL = 365 * 24 * time.Hour

// PATRevocationIdentifier returns the stable revocation-cache identifier for
// a PAT given its database ID. It is exported so the auth middleware and
// this handler agree on the key shape. The format is intentionally distinct
// from the raw-token-hash identifier used for PASETO/glst_ revocation so a
// future operator dumping the cache can tell PAT entries from session
// entries at a glance.
func PATRevocationIdentifier(id uint) string {
	return "pat:" + strconv.FormatUint(uint64(id), 10)
}

type Handler struct {
	tokensRepo repositories.PersonalAccessTokenRepository
	revocation auth.TokenRevocation
	responder  base.Responder
	audit      audit.Logger
}

func NewHandler(
	tokensRepo repositories.PersonalAccessTokenRepository,
	revocation auth.TokenRevocation,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	if revocation == nil {
		revocation = auth.NoopRevocation{}
	}

	return &Handler{
		tokensRepo: tokensRepo,
		revocation: revocation,
		responder:  responder,
		audit:      auditLogger,
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

	input := api.NewInputReader(r)

	id, err := input.ReadUint("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.NewValidationError("invalid token id"))

		return
	}

	tokens, err := h.tokensRepo.Find(ctx, filters.FindPersonalAccessTokenByIDs(id), nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find token"))

		return
	}

	if len(tokens) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("token not found"),
			http.StatusNotFound,
		))

		return
	}

	token := tokens[0]

	if token.TokenableID != session.User.ID {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("access denied"),
			http.StatusForbidden,
		))

		return
	}

	err = h.tokensRepo.Delete(ctx, id)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete token"))

		return
	}

	// Defence-in-depth: the cached repository wrapper already invalidates
	// any cached lookup for this PAT, so a subsequent middleware Find
	// returns errPATNotFound. We additionally place the PAT identifier on
	// the revocation denylist so a future regression that re-introduces a
	// stale lookup path cannot resurrect the token. The revocation entry
	// is keyed by ID (not raw secret) because the secret is no longer
	// recoverable post-delete; the auth middleware mirrors this key shape
	// for the post-validation IsRevoked probe.
	if revErr := h.revocation.Revoke(ctx, auth.TokenIdentifier(PATRevocationIdentifier(id)), patRevocationTTL); revErr != nil {
		// Non-fatal: the DB delete already strips API access. Surface the
		// error for operator visibility so they can investigate cache
		// outages.
		slog.WarnContext(ctx, "failed to record PAT revocation in denylist",
			slog.Uint64("pat_id", uint64(id)),
			slog.String("error", revErr.Error()))
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventPATRevoke, audit.CategoryTokenOp,
		"token", strconv.FormatUint(uint64(id), 10), "revoke")

	rw.WriteHeader(http.StatusNoContent)
}
