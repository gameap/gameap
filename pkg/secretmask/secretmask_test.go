package secretmask_test

import (
	"testing"

	"github.com/gameap/gameap/pkg/secretmask"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMasker_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		secrets []string
		input   string
		want    string
	}{
		{
			name:    "no_secrets_returns_input_unchanged",
			secrets: nil,
			input:   "./hlds_run +rcon_password s3cr3tpass",
			want:    "./hlds_run +rcon_password s3cr3tpass",
		},
		{
			name:    "empty_secret_ignored",
			secrets: []string{""},
			input:   "console output",
			want:    "console output",
		},
		{
			name:    "two_character_secret_replaced",
			secrets: []string{"ab"},
			input:   "abcabc",
			want:    "******c******c",
		},
		{
			name:    "single_character_secret_replaced",
			secrets: []string{"x"},
			input:   "axbxc",
			want:    "a******b******c",
		},
		{
			name:    "longer_secret_wins_over_contained_shorter_one",
			secrets: []string{"ab", "abcd"},
			input:   "value abcd end",
			want:    "value ****** end",
		},
		{
			name:    "shorter_secret_still_replaced_on_its_own",
			secrets: []string{"abcd", "ab"},
			input:   "abcd and ab",
			want:    "****** and ******",
		},
		{
			name:    "single_occurrence_replaced",
			secrets: []string{"s3cr3tpass"},
			input:   "./hlds_run -game cs +rcon_password s3cr3tpass +port 27015",
			want:    "./hlds_run -game cs +rcon_password ****** +port 27015",
		},
		{
			name:    "multiple_occurrences_replaced",
			secrets: []string{"s3cr3tpass"},
			input:   "rcon_password s3cr3tpass\nrcon_password is s3cr3tpass",
			want:    "rcon_password ******\nrcon_password is ******",
		},
		{
			name:    "multiple_secrets_replaced",
			secrets: []string{"s3cr3tpass", "another"},
			input:   "first s3cr3tpass second another",
			want:    "first ****** second ******",
		},
		{
			name:    "duplicate_secrets_deduped",
			secrets: []string{"s3cr3tpass", "s3cr3tpass"},
			input:   "value s3cr3tpass",
			want:    "value ******",
		},
		{
			name:    "unicode_secret_replaced",
			secrets: []string{"пароль123"},
			input:   "rcon_password пароль123 done",
			want:    "rcon_password ****** done",
		},
		{
			name:    "secret_inside_word_replaced",
			secrets: []string{"s3cr3tpass"},
			input:   `+rcon_password "s3cr3tpass"`,
			want:    `+rcon_password "******"`,
		},
		{
			name:    "empty_input_returns_empty",
			secrets: []string{"s3cr3tpass"},
			input:   "",
			want:    "",
		},
		{
			name:    "input_without_secret_unchanged",
			secrets: []string{"s3cr3tpass"},
			input:   "L 01/02/2026 - 12:00:00: Started map \"de_dust2\"",
			want:    "L 01/02/2026 - 12:00:00: Started map \"de_dust2\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			masker := secretmask.New(tt.secrets...)

			// ACT
			got := masker.String(tt.input)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMasker_Bytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		secrets []string
		input   []byte
		want    []byte
	}{
		{
			name:    "no_secrets_returns_input_unchanged",
			secrets: nil,
			input:   []byte("+rcon_password s3cr3tpass"),
			want:    []byte("+rcon_password s3cr3tpass"),
		},
		{
			name:    "single_occurrence_replaced",
			secrets: []string{"s3cr3tpass"},
			input:   []byte("+rcon_password s3cr3tpass"),
			want:    []byte("+rcon_password ******"),
		},
		{
			name:    "multiple_secrets_replaced",
			secrets: []string{"s3cr3tpass", "another"},
			input:   []byte("s3cr3tpass and another"),
			want:    []byte("****** and ******"),
		},
		{
			name:    "nil_input_returns_nil",
			secrets: []string{"s3cr3tpass"},
			input:   nil,
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			masker := secretmask.New(tt.secrets...)

			// ACT
			got := masker.Bytes(tt.input)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMasker_Bytes_does_not_modify_input(t *testing.T) {
	t.Parallel()
	// ARRANGE
	masker := secretmask.New("s3cr3tpass")
	input := []byte("+rcon_password s3cr3tpass")

	// ACT
	got := masker.Bytes(input)

	// ASSERT
	assert.Equal(t, []byte("+rcon_password ******"), got)
	assert.Equal(t, []byte("+rcon_password s3cr3tpass"), input)
}

func TestMasker_Bytes_returns_same_slice_when_no_match(t *testing.T) {
	t.Parallel()
	// ARRANGE
	masker := secretmask.New("s3cr3tpass")
	input := []byte("nothing to hide here")

	// ACT
	got := masker.Bytes(input)

	// ASSERT
	require.Len(t, got, len(input))
	assert.Equal(t, &input[0], &got[0], "slice must be returned as is when nothing matches")
}

func TestMasker_Empty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		masker  *secretmask.Masker
		want    bool
		input   string
		wantOut string
	}{
		{
			name:    "nil_masker_is_empty_and_noop",
			masker:  nil,
			want:    true,
			input:   "+rcon_password s3cr3tpass",
			wantOut: "+rcon_password s3cr3tpass",
		},
		{
			name:    "masker_without_secrets_is_empty",
			masker:  secretmask.New(),
			want:    true,
			input:   "+rcon_password s3cr3tpass",
			wantOut: "+rcon_password s3cr3tpass",
		},
		{
			name:    "masker_with_only_empty_secrets_is_empty",
			masker:  secretmask.New("", ""),
			want:    true,
			input:   "ab cd",
			wantOut: "ab cd",
		},
		{
			name:    "masker_with_short_secret_is_not_empty",
			masker:  secretmask.New("", "ab"),
			want:    false,
			input:   "ab cd",
			wantOut: "****** cd",
		},
		{
			name:    "masker_with_secret_is_not_empty",
			masker:  secretmask.New("s3cr3tpass"),
			want:    false,
			input:   "+rcon_password s3cr3tpass",
			wantOut: "+rcon_password ******",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			empty := tt.masker.Empty()
			masked := tt.masker.String(tt.input)
			maskedBytes := tt.masker.Bytes([]byte(tt.input))

			// ASSERT
			assert.Equal(t, tt.want, empty)
			assert.Equal(t, tt.wantOut, masked)
			assert.Equal(t, tt.wantOut, string(maskedBytes))
		})
	}
}

func TestMasker_Bytes_masksShortSecret(t *testing.T) {
	t.Parallel()
	// ARRANGE
	masker := secretmask.New("ab")

	// ACT
	got := masker.Bytes([]byte("+rcon_password ab"))

	// ASSERT
	assert.Equal(t, "+rcon_password ******", string(got))
}

func TestPlaceholder_hides_secret_length(t *testing.T) {
	t.Parallel()
	// ARRANGE
	short := secretmask.New("abcd")
	long := secretmask.New("averylongrconpassword")

	// ACT
	shortMasked := short.String("pass abcd")
	longMasked := long.String("pass averylongrconpassword")

	// ASSERT
	assert.Equal(t, "pass "+secretmask.Placeholder, shortMasked)
	assert.Equal(t, "pass "+secretmask.Placeholder, longMasked)
}
