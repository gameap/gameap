// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — these tests pin the wire/encoding
//     contract of the single-use short-lived token primitive: prefix detection
//     must route only genuine short-lived tokens to the cache validator; the
//     cache key must never contain the raw secret (only its SHA-256), so a
//     token leaked into a URL/log cannot be recovered from the cache key; and
//     the cached payload must round-trip exactly so a reconstructed session is
//     never broader (extra abilities) nor narrower than the one that minted it.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package auth

import (
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsShortLivedToken covers OWASP API2:2023: only a token that starts with
// the dedicated prefix is routed to the short-lived validator. Every other
// credential shape (and a prefix that is not at the very start) must NOT be
// treated as a short-lived token, so the auth middleware cannot be tricked
// into validating a PASETO/JWT/PAT against the short-token cache.
func TestIsShortLivedToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{
			name:  "prefixed_token_is_short_lived",
			token: "glst_x",
			want:  true,
		},
		{
			name:  "prefix_alone_with_a_secret_is_short_lived",
			token: ShortLivedTokenPrefix + "aB3dE5fG7hJ9kL1mN3pQ5rS7tU9vW1xY3zA5bC7dE9fG1hJ",
			want:  true,
		},
		{
			name:  "empty_string_is_not_short_lived",
			token: "",
			want:  false,
		},
		{
			name:  "prefix_without_trailing_underscore_is_not_short_lived",
			token: "glst",
			want:  false,
		},
		{
			name:  "leading_space_before_prefix_is_not_short_lived",
			token: " glst_x",
			want:  false,
		},
		{
			name:  "paseto_local_token_is_not_short_lived",
			token: "v4.local.ZpRdkTbKsomethingsomething",
			want:  false,
		},
		{
			name:  "paseto_public_token_is_not_short_lived",
			token: "v4.public.somethingsomething",
			want:  false,
		},
		{
			name:  "jwt_token_is_not_short_lived",
			token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.sig",
			want:  false,
		},
		{
			name:  "personal_access_token_is_not_short_lived",
			token: "13|gKwaw8PjGrkmxRgsecret",
			want:  false,
		},
		{
			name:  "prefix_in_the_middle_is_not_short_lived",
			token: "abcglst_x",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE / ACT
			got := IsShortLivedToken(tt.token)

			// ASSERT
			assert.Equal(t, tt.want, got,
				"IsShortLivedToken(%q) must report whether the dedicated prefix starts the token", tt.token)
		})
	}
}

// TestShortLivedCacheKey covers OWASP API2:2023: the cache key is namespaced
// and is the SHA-256 of ONLY the secret part (prefix stripped). The raw secret
// must never appear in the key, so a token captured from a URL/log/proxy
// cannot be turned back into a cache lookup the attacker controls.
func TestShortLivedCacheKey(t *testing.T) {
	t.Parallel()

	const keyPrefix = "auth:shorttoken:"
	secret := "aB3dE5fG7hJ9kL1mN3pQ5rS7tU9vW1xY3zA5bC7dE9fG1hJ"
	token := ShortLivedTokenPrefix + secret

	t.Run("key_is_namespaced", func(t *testing.T) {
		t.Parallel()

		// ARRANGE / ACT
		key := ShortLivedCacheKey(token)

		// ASSERT
		assert.True(t, strings.HasPrefix(key, keyPrefix),
			"the cache key must carry the auth:shorttoken: namespace, got %q", key)
	})

	t.Run("prefix_is_stripped_before_hashing", func(t *testing.T) {
		t.Parallel()

		// ARRANGE / ACT
		key := ShortLivedCacheKey(token)

		// ASSERT — only the secret (without the glst_ prefix) is hashed.
		assert.Equal(t, keyPrefix+pkgstrings.SHA256(secret), key,
			"the key must be the SHA-256 of the secret with the prefix removed")
	})

	t.Run("token_without_prefix_hashes_the_whole_string", func(t *testing.T) {
		t.Parallel()

		// ARRANGE — TrimPrefix is a no-op when the prefix is absent, so the
		// entire input is hashed. Documenting the actual behavior.
		raw := "no-prefix-here"

		// ACT
		key := ShortLivedCacheKey(raw)

		// ASSERT
		assert.Equal(t, keyPrefix+pkgstrings.SHA256(raw), key,
			"with no prefix to strip the whole input is hashed")
	})

	t.Run("derivation_is_deterministic", func(t *testing.T) {
		t.Parallel()

		// ARRANGE / ACT
		first := ShortLivedCacheKey(token)
		second := ShortLivedCacheKey(token)

		// ASSERT
		assert.Equal(t, first, second, "the same token must always derive the same key")
	})

	t.Run("different_secrets_derive_different_keys", func(t *testing.T) {
		t.Parallel()

		// ARRANGE / ACT
		keyA := ShortLivedCacheKey(ShortLivedTokenPrefix + "secret-a")
		keyB := ShortLivedCacheKey(ShortLivedTokenPrefix + "secret-b")

		// ASSERT
		assert.NotEqual(t, keyA, keyB, "distinct secrets must not collide on the same cache key")
	})

	t.Run("raw_secret_never_appears_in_the_key", func(t *testing.T) {
		t.Parallel()

		// ARRANGE / ACT
		key := ShortLivedCacheKey(token)

		// ASSERT — a secret leaked into the URL is only ever stored as a hash.
		assert.NotContains(t, key, secret,
			"the cache key must never embed the raw secret")
		assert.NotContains(t, key, token,
			"the cache key must never embed the full token")
	})
}

// TestMarshalUnmarshalShortLivedPayload_RoundTrip covers OWASP API2:2023: a
// payload (including the PAT id and abilities that scope it) must survive a
// marshal/unmarshal round-trip unchanged through both the string and []byte
// shapes the in-memory and Redis caches return — otherwise a reconstructed
// session could silently gain or lose authority.
func TestMarshalUnmarshalShortLivedPayload_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload ShortLivedPayload
	}{
		{
			name: "session_derived_payload_without_pat",
			payload: ShortLivedPayload{
				UserID: 42,
				Login:  "alice",
				Email:  "alice@example.com",
			},
		},
		{
			name: "pat_derived_payload_with_abilities",
			payload: ShortLivedPayload{
				UserID: 7,
				Login:  "bob",
				Email:  "bob@example.com",
				PATID:  99,
				Abilities: []domain.PATAbility{
					domain.PATAbilityServerList,
					domain.PATAbilityServerConsole,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			encoded, err := MarshalShortLivedPayload(tt.payload)
			require.NoError(t, err)

			// ACT — both cache backends are exercised: string (in-memory) and
			// []byte (Redis).
			fromString, errStr := UnmarshalShortLivedPayload(encoded)
			fromBytes, errBytes := UnmarshalShortLivedPayload([]byte(encoded))

			// ASSERT
			require.NoError(t, errStr, "a string value must decode")
			require.NoError(t, errBytes, "a []byte value must decode")
			assert.Equal(t, tt.payload, fromString,
				"the payload must round-trip unchanged from a string value")
			assert.Equal(t, tt.payload, fromBytes,
				"the payload must round-trip unchanged from a []byte value")
		})
	}
}

// TestMarshalShortLivedPayload_OmitsEmptyScopeFields covers OWASP API2:2023:
// a session-derived token carries no PAT id / abilities, and the JSON must
// omit them (omitempty) so the cached blob cannot be misread as carrying an
// empty (= deny-all) ability set. Also pins the stable JSON field names.
func TestMarshalShortLivedPayload_OmitsEmptyScopeFields(t *testing.T) {
	t.Parallel()

	t.Run("empty_pat_and_abilities_are_omitted", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		payload := ShortLivedPayload{UserID: 42, Login: "alice", Email: "alice@example.com"}

		// ACT
		encoded, err := MarshalShortLivedPayload(payload)

		// ASSERT
		require.NoError(t, err)
		assert.JSONEq(t, `{"user_id":42,"login":"alice","email":"alice@example.com"}`, encoded,
			"pat_id and abilities must be omitted when unset")
		assert.NotContains(t, encoded, "pat_id", "an unset PAT id must not appear in the JSON")
		assert.NotContains(t, encoded, "abilities", "an unset ability set must not appear in the JSON")
	})

	t.Run("populated_scope_fields_use_the_stable_field_names", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		payload := ShortLivedPayload{
			UserID:    7,
			Login:     "bob",
			Email:     "bob@example.com",
			PATID:     99,
			Abilities: []domain.PATAbility{domain.PATAbilityServerList},
		}

		// ACT
		encoded, err := MarshalShortLivedPayload(payload)

		// ASSERT
		require.NoError(t, err)
		assert.JSONEq(t,
			`{"user_id":7,"login":"bob","email":"bob@example.com","pat_id":99,"abilities":["server:list"]}`,
			encoded,
			"the JSON field names are part of the stable cache contract")
	})
}

// TestUnmarshalShortLivedPayload_RejectsBadInput covers OWASP API2:2023:
// the decoder must fail closed — an unexpected cache value type or a
// non-JSON/garbage value must return an error (never a zero-valued payload
// that would otherwise authenticate as user id 0).
func TestUnmarshalShortLivedPayload_RejectsBadInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       any
		wantError string
	}{
		{
			name:      "int_value_is_an_unexpected_type",
			raw:       12345,
			wantError: "unexpected short-lived token payload type",
		},
		{
			name:      "nil_value_is_an_unexpected_type",
			raw:       nil,
			wantError: "unexpected short-lived token payload type",
		},
		{
			name:      "map_value_is_an_unexpected_type",
			raw:       map[string]any{"user_id": 1},
			wantError: "unexpected short-lived token payload type",
		},
		{
			name:      "bool_value_is_an_unexpected_type",
			raw:       true,
			wantError: "unexpected short-lived token payload type",
		},
		{
			name:      "non_json_string_is_invalid",
			raw:       "not-json-at-all",
			wantError: "failed to unmarshal short-lived token payload",
		},
		{
			name:      "non_json_bytes_are_invalid",
			raw:       []byte("{ broken json"),
			wantError: "failed to unmarshal short-lived token payload",
		},
		{
			name:      "json_array_does_not_fit_the_payload_object",
			raw:       "[1,2,3]",
			wantError: "failed to unmarshal short-lived token payload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE / ACT
			payload, err := UnmarshalShortLivedPayload(tt.raw)

			// ASSERT
			require.Error(t, err, "a bad cache value must not decode silently")
			assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			assert.Equal(t, ShortLivedPayload{}, payload,
				"a failed decode must yield the zero payload, never a partial one")
		})
	}
}
