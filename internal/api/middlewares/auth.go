package middlewares

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gameap/gameap/internal/api/base"
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
	errInvalidTokenSubject      = errors.New("invalid token subject")
	errUserNotFound             = errors.New("user not found")
	errUserNotAuthenticated     = errors.New("user not authenticated")
	errAdminPermissionsRequired = errors.New("admin permissions required")
)

type tokenType int

const (
	tokenTypeUnknown tokenType = iota
	tokenTypePersonalAccess
	tokenTypeUserAuth
)

type AuthMiddleware struct {
	authService authService
	userRepo    repositories.UserRepository
	tokenRepo   repositories.PersonalAccessTokenRepository
	responder   base.Responder
}

func NewAuthMiddleware(
	authService authService,
	userRepo repositories.UserRepository,
	tokenRepo repositories.PersonalAccessTokenRepository,
	responder base.Responder,
) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
		userRepo:    userRepo,
		tokenRepo:   tokenRepo,
		responder:   responder,
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
		default:
			m.responder.WriteError(r.Context(), w, api.WrapHTTPError(
				errInvalidOrExpiredToken,
				http.StatusUnauthorized,
			))

			return
		}
		if err != nil {
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

		ctx := auth.ContextWithSession(r.Context(), session)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
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

	if dbToken.Token != pkgstrings.SHA256(tokenPart) {
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
}

func NewIsAdminMiddleware(
	rbac base.RBAC,
	responder base.Responder,
) *IsAdminMiddleware {
	return &IsAdminMiddleware{
		rbac:      rbac,
		responder: responder,
	}
}

func (m *IsAdminMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		session := auth.SessionFromContext(ctx)

		if !session.IsAuthenticated() {
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
			m.responder.WriteError(ctx, w, api.WrapHTTPError(
				errAdminPermissionsRequired,
				http.StatusForbidden,
			))

			return
		}

		next.ServeHTTP(w, r)
	})
}
