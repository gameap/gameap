package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUint64ID_Value(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   Uint64ID
		want int64
	}{
		{
			name: "zero_value",
			id:   0,
			want: 0,
		},
		{
			name: "small_value",
			id:   42,
			want: 42,
		},
		{
			name: "large_value",
			id:   4120302874985960141,
			want: 4120302874985960141,
		},
		{
			name: "max_int64",
			id:   Uint64ID(1<<63 - 1),
			want: 1<<63 - 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := test.id.Value()
			require.NoError(t, err)
			assert.Equal(t, test.want, result)
		})
	}
}

func TestUint64ID_Scan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     any
		want      Uint64ID
		wantError string
	}{
		{
			name:  "int64_zero",
			input: int64(0),
			want:  0,
		},
		{
			name:  "int64_positive",
			input: int64(42),
			want:  42,
		},
		{
			name:  "int64_large",
			input: int64(4120302874985960141),
			want:  4120302874985960141,
		},
		{
			name:  "uint64_value",
			input: uint64(123456789),
			want:  123456789,
		},
		{
			name:  "int32_value",
			input: int32(100),
			want:  100,
		},
		{
			name:  "int_value",
			input: int(200),
			want:  200,
		},
		{
			name:  "negative_int64",
			input: int64(-1),
			want:  Uint64ID(^uint64(0)), // -1 as uint64
		},
		{
			name:  "max_uint64",
			input: uint64(^uint64(0)), //nolint:unconvert
			want:  Uint64ID(^uint64(0)),
		},
		{
			name:  "nil_value",
			input: nil,
			want:  0,
		},
		{
			name:  "string_value",
			input: "123",
			want:  123,
		},
		{
			name:      "string_invalid",
			input:     "not_a_number",
			wantError: "failed to parse string to uint64",
		},
		{
			name:      "float_unsupported",
			input:     float64(1.5),
			wantError: "cannot scan float64 into Uint64ID",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var id Uint64ID
			err := id.Scan(test.input)

			if test.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, id)
		})
	}
}

func TestUint64ID_ScanValue_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   Uint64ID
	}{
		{
			name: "zero_round_trip",
			id:   0,
		},
		{
			name: "small_round_trip",
			id:   42,
		},
		{
			name: "large_round_trip",
			id:   4120302874985960141,
		},
		{
			name: "max_int64_round_trip",
			id:   Uint64ID(1<<63 - 1),
		},
		{
			name: "max_uint64_round_trip",
			id:   Uint64ID(^uint64(0)),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			value, err := test.id.Value()
			require.NoError(t, err)

			var scanned Uint64ID
			err = scanned.Scan(value)
			require.NoError(t, err)

			assert.Equal(t, test.id, scanned)
		})
	}
}
