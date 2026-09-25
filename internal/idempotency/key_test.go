package idempotency_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/idempotency"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		values    []string
		wantKey   string
		wantOK    bool
		wantError string
	}{
		{
			name:    "bare_token",
			values:  []string{"8e03978e-40d5-43e8-bc93-6894a57f9324"},
			wantKey: "8e03978e-40d5-43e8-bc93-6894a57f9324",
			wantOK:  true,
		},
		{
			name:    "structured_field_string",
			values:  []string{`"8e03978e-40d5-43e8-bc93-6894a57f9324"`},
			wantKey: "8e03978e-40d5-43e8-bc93-6894a57f9324",
			wantOK:  true,
		},
		{
			name:    "structured_field_string_with_escapes",
			values:  []string{`"order \"42\" \\ create"`},
			wantKey: `order "42" \ create`,
			wantOK:  true,
		},
		{
			name:    "surrounding_whitespace_is_trimmed",
			values:  []string{"  order-42-create\t"},
			wantKey: "order-42-create",
			wantOK:  true,
		},
		{
			name:    "key_of_max_length",
			values:  []string{strings.Repeat("k", 255)},
			wantKey: strings.Repeat("k", 255),
			wantOK:  true,
		},
		{
			name:   "absent_header",
			values: nil,
			wantOK: false,
		},
		{
			name:   "empty_value_counts_as_absent",
			values: []string{"   "},
			wantOK: false,
		},
		{
			name:   "empty_quoted_value_counts_as_absent",
			values: []string{`""`},
			wantOK: false,
		},
		{
			name:      "repeated_header",
			values:    []string{"first", "second"},
			wantError: "the header must be sent once: invalid Idempotency-Key header",
		},
		{
			name:      "key_over_max_length",
			values:    []string{strings.Repeat("k", 256)},
			wantError: "the key must be at most 255 characters long: invalid Idempotency-Key header",
		},
		{
			name:      "non_ascii_key",
			values:    []string{"ключ-42"},
			wantError: "the key must contain printable ASCII characters only: invalid Idempotency-Key header",
		},
		{
			name:      "control_character_in_key",
			values:    []string{"order\x01create"},
			wantError: "the key must contain printable ASCII characters only: invalid Idempotency-Key header",
		},
		{
			name:      "unterminated_quoted_key",
			values:    []string{`"order-42`},
			wantError: "the quoted key is not terminated: invalid Idempotency-Key header",
		},
		{
			name:      "invalid_escape_in_quoted_key",
			values:    []string{`"order\n42"`},
			wantError: "the quoted key has an invalid escape: invalid Idempotency-Key header",
		},
		{
			name:      "unescaped_quote_in_quoted_key",
			values:    []string{`"order"42"`},
			wantError: "the quoted key has an unescaped quote: invalid Idempotency-Key header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			header := http.Header{}
			for _, value := range tt.values {
				header.Add(idempotency.HeaderKey, value)
			}

			key, ok, err := idempotency.ParseKey(header)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.ErrorIs(t, err, idempotency.ErrInvalidKey)
				assert.Contains(t, err.Error(), tt.wantError)
				assert.False(t, ok)
				assert.Empty(t, key)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantKey, key)
		})
	}
}
