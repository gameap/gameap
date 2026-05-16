package auth

import (
	"encoding/json"
	"strings"

	"github.com/gameap/gameap/internal/domain"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
)

// ShortLivedTokenPrefix identifies a single-use, short-lived authorization
// token. It is deliberately distinct from the PASETO ("v4."), JWT ("eyJ") and
// Personal Access Token ("<id>|") shapes so the auth middleware can route a
// presented token to the right validator by prefix alone.
const ShortLivedTokenPrefix = "glst_"

const shortLivedCacheKeyPrefix = "auth:shorttoken:"

// IsShortLivedToken reports whether the raw token is a short-lived token.
func IsShortLivedToken(token string) bool {
	return strings.HasPrefix(token, ShortLivedTokenPrefix)
}

// ShortLivedCacheKey derives the cache key for a presented short-lived token.
// Only the SHA-256 of the secret part is ever used as the key — the secret
// itself is never stored, mirroring the Personal Access Token model.
func ShortLivedCacheKey(token string) string {
	secret := strings.TrimPrefix(token, ShortLivedTokenPrefix)

	return shortLivedCacheKeyPrefix + pkgstrings.SHA256(secret)
}

// ShortLivedPayload is the session reference cached under ShortLivedCacheKey.
// It carries just enough to rebuild the issuing session — including any PAT
// abilities — without trusting anything that arrived in the URL.
type ShortLivedPayload struct {
	UserID    uint                `json:"user_id"`
	Login     string              `json:"login"`
	Email     string              `json:"email"`
	PATID     uint                `json:"pat_id,omitempty"`
	Abilities []domain.PATAbility `json:"abilities,omitempty"`
}

// MarshalShortLivedPayload encodes the payload as a JSON string. A string
// value survives every cache backend unchanged (unlike numbers, which the
// Redis backend round-trips through float64).
func MarshalShortLivedPayload(p ShortLivedPayload) (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal short-lived token payload")
	}

	return string(data), nil
}

// UnmarshalShortLivedPayload decodes a cached value produced by
// MarshalShortLivedPayload. It accepts the string/[]byte shapes the in-memory
// and Redis caches return.
func UnmarshalShortLivedPayload(raw any) (ShortLivedPayload, error) {
	var data []byte

	switch v := raw.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return ShortLivedPayload{}, errors.New("unexpected short-lived token payload type")
	}

	var p ShortLivedPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return ShortLivedPayload{}, errors.Wrap(err, "failed to unmarshal short-lived token payload")
	}

	return p, nil
}
