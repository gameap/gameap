// Unit tests for the password-policy validator. Covers OWASP ASVS 4.0.3
// §2.1.1 (min length 12), §2.1.2 (deny passwords longer than 128) and
// §2.1.7 (reject common / breached passwords).
package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:paralleltest // subtests mutate the package-level blocklist and AllowWeakPasswords flag shared across cases
func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name      string
		password  string
		setup     func(t *testing.T)
		wantError string
	}{
		{
			name:      "empty_is_required",
			password:  "",
			wantError: "password is required",
		},
		{
			name:      "below_min_eleven_chars",
			password:  strings.Repeat("a", 11),
			wantError: "password must be at least 12 characters long",
		},
		{
			name:     "at_min_twelve_chars",
			password: strings.Repeat("a", 12),
		},
		{
			name:     "asvs_2_1_2_lower_bound_64_chars",
			password: strings.Repeat("a", 64),
		},
		{
			name:     "asvs_2_1_2_upper_bound_128_chars",
			password: strings.Repeat("a", 128),
		},
		{
			name:      "above_max_129_chars",
			password:  strings.Repeat("a", 129),
			wantError: "password must not exceed 128 characters",
		},
		{
			// "пароль" is 12 bytes in UTF-8 (6 Cyrillic runes × 2 bytes);
			// len() counts bytes so this is exactly at the ASVS §2.1.1 boundary.
			name:     "unicode_at_min_byte_length",
			password: "пароль",
		},
		{
			name:      "unicode_one_byte_short_of_min",
			password:  "пароль"[:11],
			wantError: "password must be at least 12 characters long",
		},
		{
			name:      "blocked_common_password",
			password:  "blockedforpolicytest",
			setup:     installBlocklistContaining(t, "blockedforpolicytest"),
			wantError: "password is too common",
		},
		{
			name:      "blocked_case_insensitive_uppercase_input",
			password:  "BLOCKEDFORPOLICYTEST",
			setup:     installBlocklistContaining(t, "blockedforpolicytest"),
			wantError: "password is too common",
		},
		{
			name:      "blocked_case_insensitive_mixed_input",
			password:  "BlockedForPolicyTest",
			setup:     installBlocklistContaining(t, "blockedforpolicytest"),
			wantError: "password is too common",
		},
		{
			name:     "not_blocked_when_allowweak_enabled",
			password: "blockedforpolicytest",
			setup: func(t *testing.T) {
				t.Helper()
				installBlocklistContaining(t, "blockedforpolicytest")(t)
				SetAllowWeakPasswords(true)
			},
		},
		{
			name:      "length_check_runs_before_blocklist",
			password:  "blocked",
			setup:     installBlocklistContaining(t, "blocked"),
			wantError: "password must be at least 12 characters long",
		},
		{
			name:     "empty_blocklist_is_noop",
			password: "thispasswordshouldbeacceptable",
		},
		{
			name:     "not_blocked_when_not_in_dictionary",
			password: "uniquelongpassphrase",
			setup:    installBlocklistContaining(t, "blockedforpolicytest"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}

			err := ValidatePassword(tt.password)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			assert.NoError(t, err)
		})
	}
}

//nolint:paralleltest // installs a package-level blocklist shared with other tests
func TestValidatePassword_BlockedSentinel(t *testing.T) {
	installBlocklistContaining(t, "blockedforpolicytest")(t)

	err := ValidatePassword("blockedforpolicytest")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPasswordBlocked), "callers must be able to detect ErrPasswordBlocked via errors.Is")
}

func TestValidatePassword_PolicyConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 12, MinPasswordLength, "ASVS §2.1.1 requires at least 12 characters")
	assert.Equal(t, 128, MaxPasswordLength, "ASVS §2.1.2 requires denying passwords longer than 128")
}

func installBlocklistContaining(t *testing.T, entries ...string) func(*testing.T) {
	t.Helper()

	return func(t *testing.T) {
		t.Helper()

		set := make(map[string]struct{}, len(entries))
		for _, e := range entries {
			set[e] = struct{}{}
		}

		SetPasswordBlocklist(&MapBlocklist{entries: set})
		t.Cleanup(ResetPasswordPolicy)
	}
}
