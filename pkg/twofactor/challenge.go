package twofactor

import (
	"encoding/json"
	"strings"

	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
)

// ChallengeTokenPrefix identifies the short-lived token issued by the login
// endpoint when the account has 2FA enabled. It is deliberately distinct from
// every session-token shape — PASETO ("v4."), JWT ("eyJ"), Personal Access
// Token ("<id>|") and the short-lived URL token ("glst_") — so the auth
// middleware's prefix router classifies it as unknown and rejects it with 401.
// A challenge token therefore cannot be exchanged for a session anywhere
// except the dedicated /api/auth/2fa/verify endpoint, which reads it straight
// from the cache. This scope confinement is the core security invariant of the
// two-step login.
const ChallengeTokenPrefix = "g2fa_"

const challengeCacheKeyPrefix = "auth:2fa-challenge:"

// IsChallengeToken reports whether the raw token is a 2FA challenge token.
func IsChallengeToken(token string) bool {
	return strings.HasPrefix(token, ChallengeTokenPrefix)
}

// ChallengeCacheKey derives the cache key for a presented challenge token.
// Only the SHA-256 of the secret part is used as the key — the secret itself
// is never stored, mirroring the Personal Access Token model.
func ChallengeCacheKey(token string) string {
	secret := strings.TrimPrefix(token, ChallengeTokenPrefix)

	return challengeCacheKeyPrefix + pkgstrings.SHA256(secret)
}

// ChallengePayload is the post-password state cached under ChallengeCacheKey.
// It carries just enough to mint the real session after the second factor is
// proven, plus a server-side attempt counter so wrong-code guesses cannot be
// retried indefinitely against the same challenge.
type ChallengePayload struct {
	UserID   uint   `json:"user_id"`
	Login    string `json:"login"`
	Email    string `json:"email"`
	Remember bool   `json:"remember"`
	Attempts int    `json:"attempts"`
	// ExpiresAt is the unix-second deadline of the challenge. It is carried in
	// the payload so a failed-attempt re-write can restore the cache entry
	// with the correct remaining TTL instead of extending the window.
	ExpiresAt int64 `json:"expires_at"`
}

// MarshalChallengePayload encodes the payload as a JSON string. A string value
// survives every cache backend unchanged (unlike numbers, which the Redis
// backend round-trips through float64).
func MarshalChallengePayload(p ChallengePayload) (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal 2fa challenge payload")
	}

	return string(data), nil
}

// UnmarshalChallengePayload decodes a cached value produced by
// MarshalChallengePayload. It accepts the string/[]byte shapes the in-memory
// and Redis caches return.
func UnmarshalChallengePayload(raw any) (ChallengePayload, error) {
	var data []byte

	switch v := raw.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return ChallengePayload{}, errors.New("unexpected 2fa challenge payload type")
	}

	var p ChallengePayload
	if err := json.Unmarshal(data, &p); err != nil {
		return ChallengePayload{}, errors.Wrap(err, "failed to unmarshal 2fa challenge payload")
	}

	return p, nil
}
