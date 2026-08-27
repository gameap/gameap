package auth

import (
	"encoding/json"
	"strings"

	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
)

// SSOTicketPrefix identifies a single-use ticket that logs one specific user
// into the panel. It is deliberately distinct from every session-token shape —
// PASETO ("v4."), JWT ("eyJ"), Personal Access Token ("<id>|"), the short-lived
// URL token ("glst_") and the 2FA challenge ("g2fa_") — and it is deliberately
// NOT taught to the auth middleware's prefix router. A presented ticket is
// therefore classified as an unknown token type and rejected with 401
// everywhere except the dedicated exchange endpoint, which reads it straight
// from the cache. That confinement is the core security invariant here: the
// ticket travels in a URL, so it must be worthless as a credential.
const SSOTicketPrefix = "glsso_"

const ssoTicketCacheKeyPrefix = "auth:sso-ticket:"

// IsSSOTicket reports whether the raw value is shaped like an SSO ticket.
func IsSSOTicket(ticket string) bool {
	return strings.HasPrefix(ticket, SSOTicketPrefix)
}

// SSOTicketCacheKey derives the cache key for a presented ticket. Only the
// SHA-256 of the secret part is ever stored, mirroring the Personal Access
// Token model — a cache dump does not yield usable tickets.
func SSOTicketCacheKey(ticket string) string {
	secret := strings.TrimPrefix(ticket, SSOTicketPrefix)

	return ssoTicketCacheKeyPrefix + pkgstrings.SHA256(secret)
}

// SSOTicketPayload is the state cached under SSOTicketCacheKey. It records who
// the ticket logs in and who asked for it, so the audit trail can answer "who
// granted this impersonation", not merely "someone logged in".
type SSOTicketPayload struct {
	UserID uint   `json:"user_id"`
	Login  string `json:"login"`

	// IssuerID is the administrator whose credentials minted the ticket;
	// IssuerPATID is the personal access token used, when it was a token
	// session rather than an interactive one.
	IssuerID    uint `json:"issuer_id"`
	IssuerPATID uint `json:"issuer_pat_id,omitempty"`

	// RedirectTo is validated at mint time and carried here, so the exchange
	// never has to trust a redirect target supplied by the browser.
	RedirectTo string `json:"redirect_to,omitempty"`

	// ClientIP optionally binds the ticket to the address that will redeem it.
	ClientIP string `json:"client_ip,omitempty"`

	// ExpiresAt is the unix-second deadline, carried in the payload so a cache
	// backend with loose TTL handling cannot widen the window.
	ExpiresAt int64 `json:"expires_at"`
}

// MarshalSSOTicketPayload encodes the payload as a JSON string. A string value
// survives every cache backend unchanged (unlike numbers, which the Redis
// backend round-trips through float64).
func MarshalSSOTicketPayload(p SSOTicketPayload) (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal sso ticket payload")
	}

	return string(data), nil
}

// UnmarshalSSOTicketPayload decodes a cached value produced by
// MarshalSSOTicketPayload. It accepts the string/[]byte shapes the in-memory
// and Redis caches return.
func UnmarshalSSOTicketPayload(raw any) (SSOTicketPayload, error) {
	var data []byte

	switch v := raw.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return SSOTicketPayload{}, errors.New("unexpected sso ticket payload type")
	}

	var p SSOTicketPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return SSOTicketPayload{}, errors.Wrap(err, "failed to unmarshal sso ticket payload")
	}

	return p, nil
}
