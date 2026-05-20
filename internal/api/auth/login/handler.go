package login

import (
	"context"
	"encoding/json"
	"net/http"
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
	"github.com/gameap/gameap/pkg/twofactor"
	"github.com/pkg/errors"
)

const (
	DefaultTokenDuration = 24 * time.Hour
	RememberMeDuration   = 7 * 24 * time.Hour

	// ChallengeTokenDuration is how long a 2FA challenge stays valid. Long
	// enough to open an authenticator app, short enough to bound replay of a
	// captured challenge token.
	ChallengeTokenDuration = 5 * time.Minute

	challengeSecretLength = 48
)

type Handler struct {
	userRepo    repositories.UserRepository
	responder   base.Responder
	authService auth.Service
	cache       cache.Cache
	audit       audit.Logger
	captcha     captchaVerifier
}

func NewHandler(
	authService auth.Service,
	userRepo repositories.UserRepository,
	tokenCache cache.Cache,
	responder base.Responder,
	auditLogger audit.Logger,
	captcha captchaVerifier,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		userRepo:    userRepo,
		responder:   responder,
		authService: authService,
		cache:       tokenCache,
		audit:       auditLogger,
		captcha:     captcha,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	input := &loginInput{}

	err := json.NewDecoder(r.Body).Decode(input)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	err = input.Validate()
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	// Verify the captcha before touching the user store: a bot must not be
	// able to probe account existence or burn DB lookups behind the wall.
	if h.captcha != nil && h.captcha.Enabled() {
		if err = h.captcha.Verify(ctx, input.Captcha, clientIP(ctx)); err != nil {
			h.responder.WriteError(ctx, rw, err)

			return
		}
	}

	// Find user by email or login
	var users []domain.User
	var filter *filters.FindUser

	if input.IsEmailLogin() {
		filter = &filters.FindUser{Emails: []string{input.Email}}
	} else {
		filter = &filters.FindUser{Logins: []string{input.Login}}
	}

	users, err = h.userRepo.Find(ctx, filter, nil, &filters.Pagination{
		Limit:  1,
		Offset: 0,
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find user"))

		return
	}

	if len(users) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("invalid credentials"),
			http.StatusUnauthorized,
		))

		return
	}

	user := users[0]

	needsRehash, err := auth.VerifyPassword(user.Password, input.Password)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("invalid credentials"),
			http.StatusUnauthorized,
		))

		return
	}

	// Transparently upgrade pre-§2.1.2 password hashes (raw bcrypt) to the
	// current SHA-256+bcrypt scheme on successful login. Best-effort: hashing
	// or save failures must not block authentication — the user is retried
	// on the next login.
	if needsRehash {
		if upgradedHash, hashErr := auth.HashPassword(input.Password); hashErr == nil {
			user.Password = upgradedHash
			_ = h.userRepo.Save(ctx, &user)
		}
	}

	if user.TwoFactorEnabled {
		h.issueTwoFactorChallenge(ctx, rw, &user, input.RememberMe())

		return
	}

	duration := DefaultTokenDuration
	if input.RememberMe() {
		duration = RememberMeDuration
	}

	// Generate JWT token
	token, err := h.authService.GenerateTokenForUser(&user, duration)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate token"))

		return
	}

	audit.LoginSuccess(ctx, h.audit, user.ID, user.Login)

	response := newLoginResponseFromUser(&user, token, DefaultTokenDuration)
	h.responder.Write(ctx, rw, response)
}

// issueTwoFactorChallenge is reached only after the password check passed for
// an account with 2FA enabled. It mints a single-use challenge token, stores
// the post-password state in the cache keyed by the token's hash, and returns
// the token instead of a session. No access token is issued here — the caller
// must complete /api/auth/2fa/verify. The challenge token uses a prefix the
// auth middleware does not recognise, so it cannot be replayed as a session.
func (h *Handler) issueTwoFactorChallenge(
	ctx context.Context, rw http.ResponseWriter, user *domain.User, remember bool,
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
		Remember:  remember,
		ExpiresAt: time.Now().Add(ChallengeTokenDuration).Unix(),
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to encode challenge payload"))

		return
	}

	err = h.cache.Set(
		ctx,
		twofactor.ChallengeCacheKey(challengeToken),
		encoded,
		cache.WithExpiration(ChallengeTokenDuration),
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to store challenge"),
			http.StatusInternalServerError,
		))

		return
	}

	audit.TwoFactorChallengeIssued(ctx, h.audit, user.ID, user.Login)

	h.responder.Write(ctx, rw, newTwoFactorChallengeResponse(challengeToken, ChallengeTokenDuration))
}

// clientIP returns the best-effort client IP captured by the global
// RequestContextMiddleware (it honours the trusted reverse-proxy header).
// Empty when the request did not pass through that middleware (unit tests);
// the captcha providers treat the remoteip field as optional.
func clientIP(ctx context.Context) string {
	if info := audit.RequestInfoFromContext(ctx); info != nil {
		return info.IP
	}

	return ""
}
