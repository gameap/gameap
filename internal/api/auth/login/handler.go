package login

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/mfanudge"
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
	userRepo           repositories.UserRepository
	responder          base.Responder
	authService        auth.Service
	cache              cache.Cache
	audit              audit.Logger
	captcha            captchaVerifier
	nudge              *mfanudge.Service
	rbac               adminChecker
	enrollmentTokenTTL time.Duration
}

func NewHandler(
	authService auth.Service,
	userRepo repositories.UserRepository,
	tokenCache cache.Cache,
	responder base.Responder,
	auditLogger audit.Logger,
	captcha captchaVerifier,
	nudge *mfanudge.Service,
	rbac adminChecker,
	enrollmentTokenTTL time.Duration,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		userRepo:           userRepo,
		responder:          responder,
		authService:        authService,
		cache:              tokenCache,
		audit:              auditLogger,
		captcha:            captcha,
		nudge:              nudge,
		rbac:               rbac,
		enrollmentTokenTTL: enrollmentTokenTTL,
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
		// Timing-oracle defence (ASVS §2.2.x): equalise wall-clock latency
		// between the "user exists / wrong password" path (one bcrypt
		// verify) and the "user does not exist" path. Without this, an
		// attacker can enumerate accounts by measuring login response time.
		// VerifyDummyPassword runs the same bcrypt compare against a
		// throwaway hash at the configured cost.
		auth.VerifyDummyPassword(input.Password)

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

	h.rehashPasswordIfNeeded(ctx, &user, input.Password, needsRehash)

	if user.TwoFactorEnabled {
		h.issueTwoFactorChallenge(ctx, rw, &user, input.RememberMe())

		return
	}

	h.completeLogin(ctx, rw, &user, input.RememberMe())
}

// completeLogin finishes a successful password authentication for an account
// without 2FA: it applies the admin-MFA nudge and issues either a full session
// token or, once the hard-fail threshold is crossed, an enrollment-scoped one.
func (h *Handler) completeLogin(
	ctx context.Context, rw http.ResponseWriter, user *domain.User, remember bool,
) {
	duration := DefaultTokenDuration
	if remember {
		duration = RememberMeDuration
	}

	nudge := h.evaluateMFANudge(ctx, user)

	// Hard fail: the grace window has elapsed for an admin without 2FA. Issue
	// an enrollment-scoped token instead of a full session, so the only thing
	// the bearer can do is enrol a second factor (or log out).
	if nudge != nil && nudge.HardFail {
		h.issueMFAEnrollmentSession(ctx, rw, user, nudge)

		return
	}

	token, err := h.authService.GenerateTokenForUser(user, duration)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate token"))

		return
	}

	audit.LoginSuccess(ctx, h.audit, user.ID, user.Login)

	response := newLoginResponseFromUser(user, token, DefaultTokenDuration)
	response.MFANudge = nudge
	h.responder.Write(ctx, rw, response)
}

// evaluateMFANudge resolves the caller's admin status and hands off to
// EvaluateMFANudge. It returns nil when no nudge applies (feature off,
// non-admin) — a best-effort step: an RBAC error never blocks the login.
func (h *Handler) evaluateMFANudge(ctx context.Context, user *domain.User) *mfanudge.View {
	if h.rbac == nil {
		return nil
	}

	isAdmin, err := h.rbac.Can(ctx, user.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		slog.WarnContext(ctx, "failed to check admin status for MFA nudge", "error", err)

		return nil
	}

	return EvaluateMFANudge(ctx, h.nudge, h.userRepo, user, isAdmin)
}

// EvaluateMFANudge computes the admin-MFA recommendation for a freshly
// authenticated user and persists the first-shown timestamp on first contact.
// It returns nil when no nudge applies.
//
// It is shared with the SSO ticket exchange so the two ways into a session
// cannot drift: an administrator who arrives through a billing link is subject
// to the same grace window, and the same hard fail at the end of it, as one who
// typed a password. The caller passes isAdmin because it has usually resolved
// it already.
//
// Persisting the timestamp is best-effort: a save error is logged and the
// recommendation is still returned, so a database hiccup never costs a login.
func EvaluateMFANudge(
	ctx context.Context,
	service *mfanudge.Service,
	userRepo MFANudgeSaver,
	user *domain.User,
	isAdmin bool,
) *mfanudge.View {
	if service == nil || userRepo == nil {
		return nil
	}

	rec := service.Compute(mfanudge.Snapshot{
		IsAdmin:          isAdmin,
		TwoFactorEnabled: user.TwoFactorEnabled,
		FirstShownAt:     user.MFAFirstShownAt(),
		SnoozedUntil:     user.MFASnoozedUntil(),
	})

	if rec.PersistFirstShown {
		user.SetMFAFirstShownAt(&rec.FirstShownAt)
		if saveErr := userRepo.Save(ctx, user); saveErr != nil {
			slog.WarnContext(ctx, "failed to persist MFA first-shown timestamp", "error", saveErr)
		}
	}

	return mfanudge.NewView(rec)
}

// issueMFAEnrollmentSession mints a token scoped to the 2FA-enrollment
// endpoints (ScopeMFAEnrollment) and returns it with mfa_enrollment_required
// set, so the frontend knows the admin must enrol before regaining access.
func (h *Handler) issueMFAEnrollmentSession(
	ctx context.Context, rw http.ResponseWriter, user *domain.User, nudge *mfanudge.View,
) {
	token, err := h.authService.GenerateMFAEnrollmentToken(user, h.enrollmentTokenTTL)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate enrollment token"))

		return
	}

	audit.LoginSuccess(ctx, h.audit, user.ID, user.Login)

	response := newLoginResponseFromUser(user, token, h.enrollmentTokenTTL)
	response.MFAEnrollmentRequired = true
	response.MFANudge = nudge
	h.responder.Write(ctx, rw, response)
}

// rehashPasswordIfNeeded transparently upgrades a stored password hash on
// successful login. Best-effort: failures must not block authentication, so
// errors are swallowed. Two triggers:
//  1. Pre-§2.1.2 raw-bcrypt scheme — needsRehash from VerifyPassword.
//  2. The stored hash uses a cost below the operator's configured
//     AUTH_BCRYPT_COST. We only ever raise the cost (`storedCost < active`);
//     a downgrade is ignored so a misconfigured operator who lowers the
//     config cannot weaken existing rows.
func (h *Handler) rehashPasswordIfNeeded(
	ctx context.Context, user *domain.User, password string, needsRehash bool,
) {
	rehash := needsRehash
	if !rehash {
		if storedCost, costErr := auth.HashCost(user.Password); costErr == nil {
			if storedCost < auth.ActiveBcryptCost() {
				rehash = true
			}
		}
	}

	if !rehash {
		return
	}

	if upgradedHash, hashErr := auth.HashPassword(password); hashErr == nil {
		user.Password = upgradedHash
		_ = h.userRepo.Save(ctx, user)
	}
}

// issueTwoFactorChallenge is reached only after the password check passed for
// an account with 2FA enabled. It mints a single-use challenge instead of a
// session; the caller must complete /api/auth/2fa/verify.
func (h *Handler) issueTwoFactorChallenge(
	ctx context.Context, rw http.ResponseWriter, user *domain.User, remember bool,
) {
	challengeToken, err := IssueTwoFactorChallenge(ctx, h.cache, h.audit, user, remember)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	h.responder.Write(ctx, rw, newTwoFactorChallengeResponse(challengeToken, ChallengeTokenDuration))
}

// IssueTwoFactorChallenge mints a single-use second-factor challenge for user,
// stores the post-authentication state in the cache keyed by the token's hash,
// records the TwoFactorChallengeIssued audit event, and returns the opaque
// challenge token. It is shared by the password login and the SSO ticket
// exchange so the two paths cannot drift — in particular so both leave the same
// audit trail. The token uses a prefix the auth middleware does not recognise,
// so it cannot be replayed as a session.
func IssueTwoFactorChallenge(
	ctx context.Context,
	store TwoFactorChallengeStore,
	auditLogger audit.Logger,
	user *domain.User,
	remember bool,
) (string, error) {
	secret, err := pkgstrings.CryptoRandomString(challengeSecretLength)
	if err != nil {
		return "", api.WrapHTTPError(
			errors.WithMessage(err, "failed to generate challenge token"),
			http.StatusInternalServerError,
		)
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
		return "", errors.WithMessage(err, "failed to encode challenge payload")
	}

	err = store.Set(
		ctx,
		twofactor.ChallengeCacheKey(challengeToken),
		encoded,
		cache.WithExpiration(ChallengeTokenDuration),
	)
	if err != nil {
		return "", api.WrapHTTPError(
			errors.WithMessage(err, "failed to store challenge"),
			http.StatusInternalServerError,
		)
	}

	audit.TwoFactorChallengeIssued(ctx, auditLogger, user.ID, user.Login)

	return challengeToken, nil
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
