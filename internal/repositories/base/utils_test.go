package base

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTime(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		validate func(*testing.T, time.Time)
	}{
		{
			name:    "RFC3339_format",
			input:   "2025-11-12T18:58:33Z",
			wantErr: false,
			validate: func(t *testing.T, result time.Time) {
				t.Helper()

				assert.Equal(t, 2025, result.Year())
				assert.Equal(t, time.November, result.Month())
				assert.Equal(t, 12, result.Day())
			},
		},
		{
			name:    "RFC3339Nano_format",
			input:   "2025-11-12T18:58:33.123456789Z",
			wantErr: false,
			validate: func(t *testing.T, result time.Time) {
				t.Helper()

				assert.Equal(t, 2025, result.Year())
				assert.Equal(t, 123456789, result.Nanosecond())
			},
		},
		{
			name:    "MySQL_datetime_format",
			input:   "2025-11-12 18:58:33",
			wantErr: false,
			validate: func(t *testing.T, result time.Time) {
				t.Helper()

				assert.Equal(t, 2025, result.Year())
				assert.Equal(t, time.November, result.Month())
				assert.Equal(t, 12, result.Day())
				assert.Equal(t, 18, result.Hour())
				assert.Equal(t, 58, result.Minute())
				assert.Equal(t, 33, result.Second())
			},
		},
		{
			name:    "Go_default_String_format",
			input:   "2025-11-12 18:58:33.6376205 +0000 UTC",
			wantErr: false,
			validate: func(t *testing.T, result time.Time) {
				t.Helper()

				assert.Equal(t, 2025, result.Year())
				assert.Equal(t, time.November, result.Month())
				assert.Equal(t, 12, result.Day())
				assert.Equal(t, 18, result.Hour())
				assert.Equal(t, 58, result.Minute())
				assert.Equal(t, 33, result.Second())
			},
		},
		{
			name:    "DateTime_format",
			input:   "2025-11-12 18:58:33",
			wantErr: false,
			validate: func(t *testing.T, result time.Time) {
				t.Helper()

				assert.Equal(t, 2025, result.Year())
			},
		},
		{
			name:    "T_separator_with_nanoseconds",
			input:   "2025-11-12T18:58:33.123456789",
			wantErr: false,
			validate: func(t *testing.T, result time.Time) {
				t.Helper()

				assert.Equal(t, 2025, result.Year())
			},
		},
		{
			name:    "Go_default_String_format_with_monotonic_clock",
			input:   "2025-11-26 12:59:46.682801983 +0000 UTC m=+553.748227326",
			wantErr: false,
			validate: func(t *testing.T, result time.Time) {
				t.Helper()

				assert.Equal(t, 2025, result.Year())
				assert.Equal(t, time.November, result.Month())
				assert.Equal(t, 26, result.Day())
				assert.Equal(t, 12, result.Hour())
				assert.Equal(t, 59, result.Minute())
				assert.Equal(t, 46, result.Second())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseTime(tt.input)
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}
