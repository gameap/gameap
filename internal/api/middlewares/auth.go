package middlewares

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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
	"github.com/rs/xid"
)

var (
	errMissingAuthToken         = errors.New("missing authentication token")
	errInvalidPATFormat         = errors.New("invalid personal access token format")
	errInvalidPATID             = errors.New("invalid personal access token ID")
	errPATNotFound              = errors.New("personal access token not found")
	errInvalidPAT               = errors.New("invalid personal access token")
	errUnsupportedPATType       = errors.New("unsupported personal access token type")
	errUserNotFoundForPAT       = errors.New("user not found for personal access token")
	errInvalidOrExpiredToken    = errors.New("invalid or expired token")
	errInvalidShortToken        = errors.New("invalid or expired short-lived token")
	errInvalidTokenSubject      = errors.New("invalid token subject")
	errUserNotFound             = errors.New("user not found")
	errUserNotAuthenticated     = errors.New("user not authenticated")
	errAdminPermissionsRequired = errors.New("admin permissions required")
	errTokenRevoked             = errors.New("token has been revoked")
)

type tokenType int

const (
	tokenTypeUnknown tokenType = iota
	tokenTypePersonalAccess
	tokenTypeUserAuth
	tokenTypeShortLived
)

type AuthMiddleware struct {
	authService authService
	userRepo    repositories.UserRepository
	tokenRepo   repositories.PersonalAccessTokenRepository
	revocation  auth.TokenRevocation
	cache       cache.Cache
	responder   base.Responder
	audit       audit.Logger
}

func NewAuthMiddleware(
	authService authService,
	userRepo repositories.UserRepository,
	tokenRepo repositories.PersonalAccessTokenRepository,
	revocation auth.TokenRevocation,
	tokenCache cache.Cache,
	responder base.Responder,
	auditLogger audit.Logger,
) *AuthMiddleware {
	if revocation == nil {
		revocation = auth.NoopRevocation{}
	}
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &AuthMiddleware{
		authService: authService,
		userRepo:    userRepo,
		tokenRepo:   tokenRepo,
		revocation:  revocation,
		cache:       tokenCache,
		responder:   responder,
		audit:       auditLogger,
	}
}

func (m *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return m.middleware(next, false)
}

// OptionalMiddleware allows requests to pass through even without authentication
// but still validates and adds user to context if token is present.
func (m *AuthMiddleware) OptionalMiddleware(next http.Handler) http.Handler {
	return m.middleware(next, true)
}

func (m *AuthMiddleware) middleware(next http.Handler, optional bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString := m.extractToken(r)
		if tokenString == "" {
			if optional {
				next.ServeHTTP(w, r)

				return
			}

			audit.TokenRejected(r.Context(), m.audit, "missing_token")
			m.responder.WriteError(r.Context(), w, api.WrapHTTPError(
				errMissingAuthToken,
				http.StatusUnauthorized,
			))

			return
		}
		var err error
		var session *auth.Session

		switch m.detectTokenType(tokenString) {
		case tokenTypeUserAuth:
			session, err = m.processUserAuthToken(r.Context(), tokenString)
		case tokenTypePersonalAccess:
			session, err = m.processPersonalAccessToken(r.Context(), tokenString)
		case tokenTypeShortLived:
			session, err = m.processShortLivedToken(r.Context(), tokenString)
		default:
			audit.TokenRejected(r.Context(), m.audit, "unknown_token_type")
			m.responder.WriteError(r.Context(), w, api.WrapHTTPError(
				errInvalidOrExpiredToken,
				http.StatusUnauthorized,
			))

			return
		}
		if err != nil {
			if reason, ok := authRejectReason(err); ok {
				audit.TokenRejected(r.Context(), m.audit, reason)
			}

			if optional {
				var wrappedHTTPErr *api.WrappedError
				if errors.As(err, &wrappedHTTPErr) && wrappedHTTPErr.HTTPStatus() == http.StatusUnauthorized {
					next.ServeHTTP(w, r)

					return
				}
			}

			m.responder.WriteError(r.Context(), w, err)

			return
		}

		// Revocation is checked after credentials are verified — never reveal
		// to an unauthenticated caller whether a particular token is on the
		// denylist (defence against probing the denylist as an oracle).
		revoked, revErr := m.revocation.IsRevoked(r.Context(), auth.TokenIdentifier(tokenString))
		if revErr != nil {
			m.responder.WriteError(r.Context(), w, errors.WithMessage(revErr, "failed to check token revocation"))

			return
		}
		if revoked {
			audit.TokenRejected(r.Context(), m.audit, "token_revoked")

			if optional {
				next.ServeHTTP(w, r)

				return
			}

			m.responder.WriteError(r.Context(), w, api.WrapHTTPError(
				errTokenRevoked,
				http.StatusUnauthorized,
			))

			return
		}

		ctx := auth.ContextWithSession(r.Context(), session)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authRejectReason maps a credential-processing error to a stable, non-
// sensitive audit reason token. It returns ok=false for internal errors
// (e.g. repository failures) so those are not mislabelled as auth failures.
func authRejectReason(err error) (string, bool) {
	switch {
	case errors.Is(err, errInvalidPATFormat):
		return "invalid_pat_format", true
	case errors.Is(err, errInvalidPATID):
		return "invalid_pat_id", true
	case errors.Is(err, errPATNotFound):
		return "pat_not_found", true
	case errors.Is(err, errInvalidPAT):
		return "invalid_pat", true
	case errors.Is(err, errUnsupportedPATType):
		return "unsupported_pat_type", true
	case errors.Is(err, errUserNotFoundForPAT):
		return "user_not_found_for_pat", true
	case errors.Is(err, errInvalidOrExpiredToken):
		return "invalid_or_expired_token", true
	case errors.Is(err, errInvalidShortToken):
		return "shorttoken_invalid_or_expired", true
	case errors.Is(err, errInvalidTokenSubject):
		return "invalid_token_subject", true
	case errors.Is(err, errUserNotFound):
		return "user_not_found", true
	default:
		return "", false
	}
}

func (m *AuthMiddleware) extractToken(r *http.Request) string {
	// Try to extract from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		// Check for Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return parts[1]
		}
	}

	// Try to extract from query parameter (useful for WebSocket connections)
	token := r.URL.Query().Get("token")
	if token != "" {
		return token
	}

	// Try to extract from cookie (useful for web applications)
	cookie, err := r.Cookie("token")
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	return ""
}

// detectTokenType detects the type of the provided token.
// Examples of token types:
// PASETO: v4.local.ZpRdkTbK...
// JWT: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
// Personal Access Token: 13|gKwaw8PjGrkmxRg...
func (m *AuthMiddleware) detectTokenType(token string) tokenType {
	if m.cache != nil && auth.IsShortLivedToken(token) {
		return tokenTypeShortLived
	}

	if strings.HasPrefix(token, "v4.local.") ||
		strings.HasPrefix(token, "v4.public.") ||
		strings.HasPrefix(token, "eyJ") {
		return tokenTypeUserAuth
	}

	pipeIndex := strings.Index(token, "|")
	if pipeIndex > 0 {
		prefix := token[:pipeIndex]
		if pkgstrings.IsNumeric(prefix) {
			return tokenTypePersonalAccess
		}
	}

	return tokenTypeUnknown
}

func (m *AuthMiddleware) processPersonalAccessToken(ctx context.Context, token string) (*auth.Session, error) {
	// Split token into ID and token string
	parts := strings.SplitN(token, "|", 2)
	if len(parts) != 2 {
		return nil, api.WrapHTTPError(
			errInvalidPATFormat,
			http.StatusUnauthorized,
		)
	}

	idPart := parts[0]
	tokenPart := parts[1]

	// Parse ID
	id, err := strconv.Atoi(idPart)
	if err != nil {
		return nil, api.WrapHTTPError(
			errInvalidPATID,
			http.StatusUnauthorized,
		)
	}

	if id <= 0 {
		return nil, api.WrapHTTPError(
			errInvalidPATID,
			http.StatusUnauthorized,
		)
	}

	dbTokens, err := m.tokenRepo.Find(
		ctx, filters.FindPersonalAccessTokenByIDs(uint(id)), nil, &filters.Pagination{Limit: 1},
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load personal access token from repository")
	}

	if len(dbTokens) == 0 {
		return nil, api.WrapHTTPError(
			errPATNotFound,
			http.StatusUnauthorized,
		)
	}

	dbToken := &dbTokens[0]

	if subtle.ConstantTimeCompare([]byte(dbToken.Token), []byte(pkgstrings.SHA256(tokenPart))) != 1 {
		return nil, api.WrapHTTPError(
			errInvalidPAT,
			http.StatusUnauthorized,
		)
	}

	if dbToken.TokenableType != domain.EntityTypeUser {
		return nil, api.WrapHTTPError(
			errUnsupportedPATType,
			http.StatusUnauthorized,
		)
	}

	// Load user from repository
	users, err := m.userRepo.Find(
		ctx,
		filters.FindUserByIDs(dbToken.TokenableID),
		nil,
		&filters.Pagination{Limit: 1},
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load user by personal access token")
	}

	if len(users) == 0 {
		return nil, api.WrapHTTPError(
			errUserNotFoundForPAT,
			http.StatusUnauthorized,
		)
	}

	user := &users[0]

	return &auth.Session{
		ID:    xid.New().String(),
		Login: user.Login,
		Email: user.Email,
		User:  user,
		Token: dbToken,
	}, nil
}

// processShortLivedToken validates a single-use short-lived token. The cache
// entry is deleted before the session is built so a replayed token — and any
// concurrent reuse race — loses. The reconstructed session inherits the PAT
// abilities of the session that minted it (if any), so a short-lived token is
// never broader than its origin.
func (m *AuthMiddleware) processShortLivedToken(
	ctx context.Context, token string,
) (*auth.Session, error) {
	key := auth.ShortLivedCacheKey(token)

	raw, err := m.cache.Get(ctx, key)
	if err != nil {
		if errors.Is(err, cache.ErrNotFound) {
			return nil, api.WrapHTTPError(
				errInvalidShortToken,
				http.StatusUnauthorized,
			)
		}

		return nil, errors.WithMessage(err, "failed to load short-lived token from cache")
	}

	if delErr := m.cache.Delete(ctx, key); delErr != nil {
		slog.WarnContext(ctx, "failed to delete consumed short-lived token", "error", delErr)
	}

	payload, err := auth.UnmarshalShortLivedPayload(raw)
	if err != nil {
		return nil, api.WrapHTTPError(
			errInvalidShortToken,
			http.StatusUnauthorized,
		)
	}

	users, err := m.userRepo.Find(
		ctx,
		filters.FindUserByIDs(payload.UserID),
		nil,
		&filters.Pagination{Limit: 1},
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load user for short-lived token")
	}

	if len(users) == 0 {
		return nil, api.WrapHTTPError(
			errUserNotFound,
			http.StatusUnauthorized,
		)
	}

	user := &users[0]

	session := &auth.Session{
		ID:         xid.New().String(),
		Login:      user.Login,
		Email:      user.Email,
		User:       user,
		ShortLived: true,
	}

	if payload.PATID != 0 {
		abilities := payload.Abilities
		session.Token = &domain.PersonalAccessToken{
			ID:        payload.PATID,
			Abilities: &abilities,
		}
	}

	return session, nil
}

// processUserAuthToken processes standard user authentication tokens (PASETO, JWT).
func (m *AuthMiddleware) processUserAuthToken(ctx context.Context, token string) (*auth.Session, error) {
	claims, err := m.authService.ValidateToken(token)
	if err != nil {
		return nil, api.WrapHTTPError(
			errInvalidOrExpiredToken,
			http.StatusUnauthorized,
		)
	}

	subject, err := claims.GetSubject()
	if err != nil {
		return nil, api.WrapHTTPError(
			errInvalidTokenSubject,
			http.StatusUnauthorized,
		)
	}

	// Parse "user:login:..." from claims.Subject
	if !strings.HasPrefix(subject, "user:login:") {
		return nil, api.WrapHTTPError(
			errInvalidTokenSubject,
			http.StatusUnauthorized,
		)
	}

	login := strings.TrimPrefix(subject, "user:login:")
	if login == "" {
		return nil, api.WrapHTTPError(
			errInvalidTokenSubject,
			http.StatusUnauthorized,
		)
	}

	// Load user from repository
	users, err := m.userRepo.Find(
		ctx,
		&filters.FindUser{Logins: []string{login}},
		nil,
		&filters.Pagination{Limit: 1},
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load user by login from token")
	}

	if len(users) == 0 {
		return nil, api.WrapHTTPError(
			errUserNotFound,
			http.StatusUnauthorized,
		)
	}

	user := &users[0]

	return &auth.Session{
		ID:    xid.New().String(),
		Login: user.Login,
		Email: user.Email,
		User:  user,
	}, nil
}

type IsAdminMiddleware struct {
	rbac      base.RBAC
	responder base.Responder
	audit     audit.Logger
}

func NewIsAdminMiddleware(
	rbac base.RBAC,
	responder base.Responder,
	auditLogger audit.Logger,
) *IsAdminMiddleware {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &IsAdminMiddleware{
		rbac:      rbac,
		responder: responder,
		audit:     auditLogger,
	}
}

func (m *IsAdminMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		session := auth.SessionFromContext(ctx)

		if !session.IsAuthenticated() {
			audit.AccessDenied(ctx, m.audit, "admin", "", "not_authenticated")
			m.responder.WriteError(ctx, w, api.WrapHTTPError(
				errUserNotAuthenticated,
				http.StatusUnauthorized,
			))

			return
		}

		isAdmin, err := m.rbac.Can(ctx, session.User.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
		if err != nil {
			m.responder.WriteError(ctx, w, api.WrapHTTPError(
				errors.WithMessage(err, "failed to check admin status"),
				http.StatusInternalServerError,
			))

			return
		}

		if !isAdmin {
			audit.AccessDenied(ctx, m.audit, "admin", "", "admin_required")
			m.responder.WriteError(ctx, w, api.WrapHTTPError(
				errAdminPermissionsRequired,
				http.StatusForbidden,
			))

			return
		}

		next.ServeHTTP(w, r)
	})
}
