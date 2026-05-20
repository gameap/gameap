// Unit tests for the password-policy validator. Covers OWASP ASVS 4.0.3
// §2.1.1 (min length 12) and §2.1.2 (deny passwords longer than 128).
package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name      string
		password  string
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

func TestValidatePassword_PolicyConstants(t *testing.T) {
	assert.Equal(t, 12, MinPasswordLength, "ASVS §2.1.1 requires at least 12 characters")
	assert.Equal(t, 128, MaxPasswordLength, "ASVS §2.1.2 requires denying passwords longer than 128")
}
