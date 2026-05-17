package twofactorverify

import (
	"context"
	"encoding/json"
	"net/http"
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
	"github.com/gameap/gameap/pkg/twofactor"
	"github.com/pkg/errors"
)

// maxVerifyAttempts is the wrong-code budget per challenge. The challenge is
// invalidated on the (maxVerifyAttempts)-th miss, so a stolen challenge token
// cannot be brute-forced even within its TTL. A per-IP limiter still wraps the
// route as defence in depth.
const maxVerifyAttempts = 5

var (
	errInvalidChallenge = errors.New("invalid or expired challenge")
	errInvalidCode      = errors.New("invalid verification code")
)

type Handler struct {
	userRepo    repositories.UserRepository
	authService auth.Service
	twoFactor   twoFactorValidator
	cache       tokenCache
	responder   base.Responder
	audit       audit.Logger
}

func NewHandler(
	authService auth.Service,
	userRepo repositories.UserRepository,
	twoFactor twoFactorValidator,
	tokenCache tokenCache,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		userRepo:    userRepo,
		authService: authService,
		twoFactor:   twoFactor,
		cache:       tokenCache,
		responder:   responder,
		audit:       auditLogger,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	input := &verifyInput{}
	if err := json.NewDecoder(r.Body).Decode(input); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	if err := input.Validate(); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	// Reject anything that is not shaped like a challenge token before it ever
	// touches the cache — a session/PAT/short-lived token must never be
	// accepted here.
	if !twofactor.IsChallengeToken(input.ChallengeToken) {
		audit.TwoFactorVerifyFailure(ctx, h.audit, "", "invalid_challenge_format")
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(errInvalidChallenge, http.StatusUnauthorized))

		return
	}

	key := twofactor.ChallengeCacheKey(input.ChallengeToken)

	payload, ok := h.loadChallenge(ctx, rw, key)
	if !ok {
		return
	}

	user, ok := h.loadUser(ctx, rw, key, payload)
	if !ok {
		return
	}

	method, verified, err := h.verifyCode(ctx, user, input.Code)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to verify code"))

		return
	}

	if !verified {
		h.handleFailedAttempt(ctx, key, payload)
		audit.TwoFactorVerifyFailure(ctx, h.audit, payload.Login, "invalid_code")
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(errInvalidCode, http.StatusUnauthorized))

		return
	}

	if delErr := h.cache.Delete(ctx, key); delErr != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(delErr, "failed to consume challenge"))

		return
	}

	duration := login.DefaultTokenDuration
	if payload.Remember {
		duration = login.RememberMeDuration
	}

	token, err := h.authService.GenerateTokenForUser(user, duration)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate token"))

		return
	}

	audit.TwoFactorVerifySuccess(ctx, h.audit, user.ID, user.Login, method)
	audit.LoginSuccess(ctx, h.audit, user.ID, user.Login)

	h.responder.Write(ctx, rw, newVerifyResponse(user, token, duration))
}

func (h *Handler) loadChallenge(
	ctx context.Context, rw http.ResponseWriter, key string,
) (twofactor.ChallengePayload, bool) {
	raw, err := h.cache.Get(ctx, key)
	if err != nil {
		if errors.Is(err, cache.ErrNotFound) {
			audit.TwoFactorVerifyFailure(ctx, h.audit, "", "challenge_not_found")
			h.responder.WriteError(ctx, rw, api.WrapHTTPError(errInvalidChallenge, http.StatusUnauthorized))

			return twofactor.ChallengePayload{}, false
		}

		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to load challenge"))

		return twofactor.ChallengePayload{}, false
	}

	payload, err := twofactor.UnmarshalChallengePayload(raw)
	if err != nil {
		_ = h.cache.Delete(ctx, key)
		audit.TwoFactorVerifyFailure(ctx, h.audit, "", "challenge_corrupt")
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(errInvalidChallenge, http.StatusUnauthorized))

		return twofactor.ChallengePayload{}, false
	}

	return payload, true
}

func (h *Handler) loadUser(
	ctx context.Context, rw http.ResponseWriter, key string, payload twofactor.ChallengePayload,
) (*domain.User, bool) {
	users, err := h.userRepo.Find(
		ctx, filters.FindUserByIDs(payload.UserID), nil, &filters.Pagination{Limit: 1},
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to load user for challenge"))

		return nil, false
	}

	if len(users) == 0 || !users[0].TwoFactorEnabled || users[0].TwoFactorSecret == nil {
		_ = h.cache.Delete(ctx, key)
		audit.TwoFactorVerifyFailure(ctx, h.audit, payload.Login, "user_not_eligible")
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(errInvalidChallenge, http.StatusUnauthorized))

		return nil, false
	}

	return &users[0], true
}

// verifyCode tries the TOTP secret first, then the recovery codes. On success
// the consumed state (last TOTP step or the spent recovery code) is persisted
// so it cannot be replayed.
func (h *Handler) verifyCode(
	ctx context.Context, user *domain.User, code string,
) (method string, ok bool, err error) {
	if user.TwoFactorSecret != nil {
		valid, step, vErr := h.twoFactor.ValidateTOTP(*user.TwoFactorSecret, code, user.TwoFactorLastUsedStep)
		if vErr != nil {
			return "", false, vErr
		}
		if valid {
			usedStep := step
			user.TwoFactorLastUsedStep = &usedStep
			if saveErr := h.userRepo.Save(ctx, user); saveErr != nil {
				return "", false, errors.WithMessage(saveErr, "failed to persist totp step")
			}

			return "totp", true, nil
		}
	}

	if user.TwoFactorRecoveryCodes != nil {
		updated, used, cErr := h.twoFactor.ConsumeRecoveryCode(*user.TwoFactorRecoveryCodes, code)
		if cErr != nil {
			return "", false, cErr
		}
		if used {
			user.TwoFactorRecoveryCodes = &updated
			if saveErr := h.userRepo.Save(ctx, user); saveErr != nil {
				return "", false, errors.WithMessage(saveErr, "failed to persist recovery code use")
			}

			return "recovery_code", true, nil
		}
	}

	return "", false, nil
}

// handleFailedAttempt increments the per-challenge miss counter and rewrites
// the cache entry with the *remaining* TTL (never extending it). The challenge
// is destroyed once the budget is exhausted or the deadline has passed.
func (h *Handler) handleFailedAttempt(
	ctx context.Context, key string, payload twofactor.ChallengePayload,
) {
	remaining := payload.ExpiresAt - time.Now().Unix()
	if remaining <= 0 {
		_ = h.cache.Delete(ctx, key)

		return
	}

	payload.Attempts++
	if payload.Attempts >= maxVerifyAttempts {
		_ = h.cache.Delete(ctx, key)

		return
	}

	encoded, err := twofactor.MarshalChallengePayload(payload)
	if err != nil {
		_ = h.cache.Delete(ctx, key)

		return
	}

	_ = h.cache.Set(ctx, key, encoded, cache.WithExpiration(time.Duration(remaining)*time.Second))
}
