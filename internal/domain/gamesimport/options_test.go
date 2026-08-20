package gamesimport

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportOptions_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      *Options
		wantError string
	}{
		{
			name: "nil_options_is_valid",
			opts: nil,
		},
		{
			name: "empty_options_is_valid",
			opts: &Options{},
		},
		{
			name: "valid_code_only",
			opts: &Options{
				Code: new("test_game"),
			},
		},
		{
			name: "valid_name_only",
			opts: &Options{
				Name: new("Test Game"),
			},
		},
		{
			name: "valid_code_and_name",
			opts: &Options{
				Code: new("test"),
				Name: new("Test Game"),
			},
		},
		{
			name: "valid_code_min_length",
			opts: &Options{
				Code: new("ab"),
			},
		},
		{
			name: "valid_code_max_length",
			opts: &Options{
				Code: new("1234567890123456"),
			},
		},
		{
			name: "valid_name_min_length",
			opts: &Options{
				Name: new("AB"),
			},
		},
		{
			name: "valid_code_with_underscores",
			opts: &Options{
				Code: new("my_game_code"),
			},
		},
		{
			name: "valid_code_with_hyphens",
			opts: &Options{
				Code: new("my-game-code"),
			},
		},
		{
			name: "valid_code_with_numbers",
			opts: &Options{
				Code: new("game123"),
			},
		},
		{
			name:      "code_too_short",
			opts:      &Options{Code: new("a")},
			wantError: "code must be between 2 and 16 characters",
		},
		{
			name:      "code_too_long",
			opts:      &Options{Code: new("12345678901234567")},
			wantError: "code must be between 2 and 16 characters",
		},
		{
			name:      "code_with_uppercase",
			opts:      &Options{Code: new("Test")},
			wantError: "code must match pattern",
		},
		{
			name:      "code_with_spaces",
			opts:      &Options{Code: new("test game")},
			wantError: "code must match pattern",
		},
		{
			name:      "code_with_special_chars",
			opts:      &Options{Code: new("test@game")},
			wantError: "code must match pattern",
		},
		{
			name:      "name_too_short",
			opts:      &Options{Name: new("A")},
			wantError: "name must be between 2 and 128 characters",
		},
		{
			name:      "name_too_long",
			opts:      &Options{Name: new(strings.Repeat("a", 129))},
			wantError: "name must be between 2 and 128 characters",
		},
		{
			name: "valid_code_with_invalid_name_returns_error",
			opts: &Options{
				Code: new("valid"),
				Name: new("A"),
			},
			wantError: "name must be between 2 and 128 characters",
		},
		{
			name: "invalid_code_with_valid_name_returns_error",
			opts: &Options{
				Code: new("a"),
				Name: new("Valid Name"),
			},
			wantError: "code must be between 2 and 16 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.opts.Validate()

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestImportOptions_IsEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opts     *Options
		expected bool
	}{
		{
			name:     "nil_options",
			opts:     nil,
			expected: true,
		},
		{
			name:     "empty_options",
			opts:     &Options{},
			expected: true,
		},
		{
			name: "only_code_set",
			opts: &Options{
				Code: new("test"),
			},
			expected: false,
		},
		{
			name: "only_name_set",
			opts: &Options{
				Name: new("Test"),
			},
			expected: false,
		},
		{
			name: "both_set",
			opts: &Options{
				Code: new("test"),
				Name: new("Test"),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.opts.IsEmpty()
			assert.Equal(t, tt.expected, result)
		})
	}
}
