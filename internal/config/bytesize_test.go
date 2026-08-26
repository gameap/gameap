package config_test

import (
	"testing"

	"github.com/gameap/gameap/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestByteSize_UnmarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		want      uint64
		wantError string
	}{
		{name: "plain_bytes", input: "8388608", want: 8388608},
		{name: "single_byte", input: "1", want: 1},
		{name: "zero", input: "0", want: 0},
		{name: "explicit_b", input: "100B", want: 100},
		{name: "kilo_short", input: "8K", want: 8 * 1024},
		{name: "kilo_long", input: "8KB", want: 8 * 1024},
		{name: "kilo_iec", input: "8KiB", want: 8 * 1024},
		{name: "mega_short", input: "8M", want: 8 * 1024 * 1024},
		{name: "mega_long", input: "16MB", want: 16 * 1024 * 1024},
		{name: "mega_iec", input: "16MiB", want: 16 * 1024 * 1024},
		{name: "giga_short", input: "16G", want: 16 * 1024 * 1024 * 1024},
		{name: "giga_long", input: "16GB", want: 16 * 1024 * 1024 * 1024},
		{name: "giga_iec", input: "16GiB", want: 16 * 1024 * 1024 * 1024},
		{name: "tera_short", input: "2T", want: 2 * 1024 * 1024 * 1024 * 1024},
		{name: "tera_iec", input: "2TiB", want: 2 * 1024 * 1024 * 1024 * 1024},
		{name: "peta_short", input: "1P", want: 1 << 50},
		{name: "with_whitespace", input: "  16 MB  ", want: 16 * 1024 * 1024},
		{name: "lowercase", input: "8mb", want: 8 * 1024 * 1024},
		{name: "decimal_value", input: "1.5M", want: uint64(1.5 * 1024 * 1024)},
		{name: "thousand_kb", input: "1000KB", want: 1000 * 1024},

		{name: "empty_string", input: "", wantError: "empty"},
		{name: "whitespace_only", input: "   ", wantError: "empty"},
		{name: "non_numeric", input: "abc", wantError: "invalid byte size"},
		{name: "unknown_suffix", input: "8Q", wantError: "invalid byte size"},
		{name: "negative", input: "-5MB", wantError: "invalid byte size"},
		{name: "exa_unsupported", input: "1EB", wantError: "invalid byte size"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			var bs config.ByteSize

			// ACT
			err := bs.UnmarshalText([]byte(tt.input))

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, bs.Uint64())
		})
	}
}

func TestByteSize_Uint64_ReturnsRawValue(t *testing.T) {
	t.Parallel()

	// ARRANGE
	bs := config.ByteSize(42)

	// ACT
	got := bs.Uint64()

	// ASSERT
	assert.Equal(t, uint64(42), got)
}
