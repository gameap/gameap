package flexible

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlexibleTime_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		jsonStr  string
		wantErr  bool
		validate func(*testing.T, time.Time)
	}{
		{
			name:    "RFC3339_format",
			jsonStr: `"2025-10-20T20:28:10Z"`,
			wantErr: false,
			validate: func(t *testing.T, tt time.Time) {
				t.Helper()

				assert.Equal(t, 2025, tt.Year())
				assert.Equal(t, time.October, tt.Month())
				assert.Equal(t, 20, tt.Day())
				assert.Equal(t, 20, tt.Hour())
				assert.Equal(t, 28, tt.Minute())
				assert.Equal(t, 10, tt.Second())
			},
		},
		{
			name:    "RFC3339_with_timezone_offset",
			jsonStr: `"2025-10-20T20:28:10+05:00"`,
			wantErr: false,
			validate: func(t *testing.T, tt time.Time) {
				t.Helper()

				assert.Equal(t, 2025, tt.Year())
				assert.Equal(t, time.October, tt.Month())
				assert.Equal(t, 20, tt.Day())
			},
		},
		{
			name:    "MySQL_datetime_format",
			jsonStr: `"2025-10-20 20:28:10"`,
			wantErr: false,
			validate: func(t *testing.T, tt time.Time) {
				t.Helper()

				assert.Equal(t, 2025, tt.Year())
				assert.Equal(t, time.October, tt.Month())
				assert.Equal(t, 20, tt.Day())
				assert.Equal(t, 20, tt.Hour())
				assert.Equal(t, 28, tt.Minute())
				assert.Equal(t, 10, tt.Second())
			},
		},
		{
			name:    "ISO_8601_without_timezone",
			jsonStr: `"2025-10-20T20:28:10"`,
			wantErr: false,
			validate: func(t *testing.T, tt time.Time) {
				t.Helper()

				assert.Equal(t, 2025, tt.Year())
				assert.Equal(t, time.October, tt.Month())
				assert.Equal(t, 20, tt.Day())
			},
		},
		{
			name:    "format_with_timezone",
			jsonStr: `"2025-10-20 20:28:10 -0700"`,
			wantErr: false,
			validate: func(t *testing.T, tt time.Time) {
				t.Helper()

				assert.Equal(t, 2025, tt.Year())
				assert.Equal(t, time.October, tt.Month())
				assert.Equal(t, 20, tt.Day())
			},
		},
		{
			name:    "RFC3339Nano_format",
			jsonStr: `"2025-10-20T20:28:10.123456789Z"`,
			wantErr: false,
			validate: func(t *testing.T, tt time.Time) {
				t.Helper()

				assert.Equal(t, 2025, tt.Year())
				assert.Equal(t, time.October, tt.Month())
				assert.Equal(t, 20, tt.Day())
				assert.Equal(t, 20, tt.Hour())
				assert.Equal(t, 28, tt.Minute())
				assert.Equal(t, 10, tt.Second())
				assert.Equal(t, 123456789, tt.Nanosecond())
			},
		},
		{
			name:    "datetime_with_nanoseconds",
			jsonStr: `"2025-10-20 20:28:10.999999999"`,
			wantErr: false,
			validate: func(t *testing.T, tt time.Time) {
				t.Helper()

				assert.Equal(t, 2025, tt.Year())
				assert.Equal(t, time.October, tt.Month())
				assert.Equal(t, 20, tt.Day())
				assert.Equal(t, 20, tt.Hour())
				assert.Equal(t, 28, tt.Minute())
				assert.Equal(t, 10, tt.Second())
				assert.Equal(t, 999999999, tt.Nanosecond())
			},
		},
		{
			name:    "ISO_8601_with_nanoseconds",
			jsonStr: `"2025-10-20T20:28:10.999999999"`,
			wantErr: false,
			validate: func(t *testing.T, tt time.Time) {
				t.Helper()

				assert.Equal(t, 2025, tt.Year())
				assert.Equal(t, time.October, tt.Month())
				assert.Equal(t, 20, tt.Day())
				assert.Equal(t, 20, tt.Hour())
				assert.Equal(t, 28, tt.Minute())
				assert.Equal(t, 10, tt.Second())
				assert.Equal(t, 999999999, tt.Nanosecond())
			},
		},
		{
			name:    "datetime_with_nanoseconds_and_timezone",
			jsonStr: `"2025-10-20 20:28:10.9999999 -0700 MST"`,
			wantErr: false,
			validate: func(t *testing.T, tt time.Time) {
				t.Helper()

				assert.Equal(t, 2025, tt.Year())
				assert.Equal(t, time.October, tt.Month())
				assert.Equal(t, 20, tt.Day())
				assert.Equal(t, 20, tt.Hour())
				assert.Equal(t, 28, tt.Minute())
				assert.Equal(t, 10, tt.Second())
			},
		},
		{
			name:    "date_only_format",
			jsonStr: `"2025-10-20"`,
			wantErr: true,
		},
		{
			name:    "invalid_json_string",
			jsonStr: `"not a date"`,
			wantErr: true,
		},
		{
			name:    "invalid_json_syntax",
			jsonStr: `{invalid}`,
			wantErr: true,
		},
		{
			name:    "invalid_date_values",
			jsonStr: `"2025-13-45 25:70:99"`,
			wantErr: true,
		},
		{
			name:    "empty_string",
			jsonStr: `""`,
			wantErr: true,
		},
		{
			name:    "null_value",
			jsonStr: `null`,
			wantErr: false,
			validate: func(t *testing.T, tt time.Time) {
				t.Helper()

				assert.True(t, tt.IsZero(), "null should unmarshal to zero time")
			},
		},
		{
			name:    "number_instead_of_string",
			jsonStr: `12345`,
			wantErr: true,
		},
		{
			name:    "boolean_instead_of_string",
			jsonStr: `true`,
			wantErr: true,
		},
		{
			name:    "object_instead_of_string",
			jsonStr: `{"time": "2025-10-20"}`,
			wantErr: true,
		},
		{
			name:    "array_instead_of_string",
			jsonStr: `["2025-10-20"]`,
			wantErr: true,
		},
		{
			name:    "partial_datetime",
			jsonStr: `"2025-10-20T20:28"`,
			wantErr: true,
		},
		{
			name:    "wrong_separator",
			jsonStr: `"2025/10/20 20:28:10"`,
			wantErr: true,
		},
		{
			name:    "time_only",
			jsonStr: `"20:28:10"`,
			wantErr: true,
		},
		{
			name:    "unix_timestamp_string",
			jsonStr: `"1729456090"`,
			wantErr: true,
		},
		{
			name:    "malformed_timezone",
			jsonStr: `"2025-10-20T20:28:10+99:99"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var ft Time
			err := json.Unmarshal([]byte(tt.jsonStr), &ft)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, ft.Time)
				}
			}
		})
	}
}

func TestFlexibleTime_MarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		time     Time
		expected string
	}{
		{
			name:     "standard_UTC_time",
			time:     Time{Time: time.Date(2025, 10, 20, 20, 28, 10, 0, time.UTC)},
			expected: `"2025-10-20T20:28:10Z"`,
		},
		{
			name:     "midnight_UTC",
			time:     Time{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
			expected: `"2025-01-01T00:00:00Z"`,
		},
		{
			name:     "end_of_day",
			time:     Time{Time: time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)},
			expected: `"2025-12-31T23:59:59Z"`,
		},
		{
			name:     "with_timezone_offset",
			time:     Time{Time: time.Date(2025, 10, 20, 20, 28, 10, 0, time.FixedZone("EST", -5*60*60))},
			expected: `"2025-10-20T20:28:10-05:00"`,
		},
		{
			name:     "leap_year_date",
			time:     Time{Time: time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)},
			expected: `"2024-02-29T12:00:00Z"`,
		},
		{
			name:     "zero_time",
			time:     Time{Time: time.Time{}},
			expected: `"0001-01-01T00:00:00Z"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.time)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, string(data))
		})
	}
}

func TestFlexibleTime_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		original string
	}{
		{
			name:     "RFC3339_roundtrip",
			original: `"2025-10-20T20:28:10Z"`,
		},
		{
			name:     "MySQL_datetime_roundtrip",
			original: `"2025-10-20 20:28:10"`,
		},
		{
			name:     "ISO_8601_roundtrip",
			original: `"2025-10-20T20:28:10"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var ft Time
			err := json.Unmarshal([]byte(tt.original), &ft)
			require.NoError(t, err)

			marshaled, err := json.Marshal(ft)
			require.NoError(t, err)

			var ft2 Time
			err = json.Unmarshal(marshaled, &ft2)
			require.NoError(t, err)

			assert.True(t, ft.Equal(ft2.Time), "Times should be equal after round trip")
		})
	}
}

func TestFlexibleTime_StructMarshaling(t *testing.T) {
	t.Parallel()

	type TestStruct struct {
		CreatedAt Time `json:"created_at"`
		UpdatedAt Time `json:"updated_at"`
	}

	t.Run("marshal_struct_with_flexible_time", func(t *testing.T) {
		t.Parallel()

		ts := TestStruct{
			CreatedAt: Time{Time: time.Date(2025, 10, 20, 10, 0, 0, 0, time.UTC)},
			UpdatedAt: Time{Time: time.Date(2025, 10, 21, 15, 30, 0, 0, time.UTC)},
		}

		data, err := json.Marshal(ts)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"created_at":"2025-10-20T10:00:00Z"`)
		assert.Contains(t, string(data), `"updated_at":"2025-10-21T15:30:00Z"`)
	})

	t.Run("unmarshal_struct_with_flexible_time_mixed_formats", func(t *testing.T) {
		t.Parallel()

		jsonData := `{
			"created_at": "2025-10-20 10:00:00",
			"updated_at": "2025-10-21T15:30:00Z"
		}`

		var ts TestStruct
		err := json.Unmarshal([]byte(jsonData), &ts)
		require.NoError(t, err)

		assert.Equal(t, 2025, ts.CreatedAt.Year())
		assert.Equal(t, time.October, ts.CreatedAt.Month())
		assert.Equal(t, 20, ts.CreatedAt.Day())

		assert.Equal(t, 2025, ts.UpdatedAt.Year())
		assert.Equal(t, time.October, ts.UpdatedAt.Month())
		assert.Equal(t, 21, ts.UpdatedAt.Day())
	})
}

func TestFlexibleTime_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("very_old_date", func(t *testing.T) {
		t.Parallel()

		ft := Time{Time: time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)}
		data, err := json.Marshal(ft)
		require.NoError(t, err)

		var ft2 Time
		err = json.Unmarshal(data, &ft2)
		require.NoError(t, err)
		assert.True(t, ft.Equal(ft2.Time))
	})

	t.Run("far_future_date", func(t *testing.T) {
		t.Parallel()

		ft := Time{Time: time.Date(2100, 12, 31, 23, 59, 59, 0, time.UTC)}
		data, err := json.Marshal(ft)
		require.NoError(t, err)

		var ft2 Time
		err = json.Unmarshal(data, &ft2)
		require.NoError(t, err)
		assert.True(t, ft.Equal(ft2.Time))
	})

	t.Run("timezone_conversion_preserves_instant", func(t *testing.T) {
		t.Parallel()

		utc := Time{Time: time.Date(2025, 10, 20, 20, 0, 0, 0, time.UTC)}
		est := Time{Time: time.Date(2025, 10, 20, 15, 0, 0, 0, time.FixedZone("EST", -5*60*60))}

		utcData, err := json.Marshal(utc)
		require.NoError(t, err)

		estData, err := json.Marshal(est)
		require.NoError(t, err)

		var utcParsed, estParsed Time
		require.NoError(t, json.Unmarshal(utcData, &utcParsed))
		require.NoError(t, json.Unmarshal(estData, &estParsed))

		assert.True(t, utcParsed.Equal(estParsed.Time), "Same instant should be equal regardless of timezone")
	})
}

func TestFlexibleTime_AllSupportedFormats(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2025, 10, 20, 20, 28, 10, 123456789, time.UTC)

	tests := []struct {
		name   string
		input  string
		wantOk bool
	}{
		{
			name:   "RFC3339",
			input:  baseTime.Format(time.RFC3339),
			wantOk: true,
		},
		{
			name:   "MySQL_datetime",
			input:  baseTime.Format("2006-01-02 15:04:05"),
			wantOk: true,
		},
		{
			name:   "RFC3339Nano",
			input:  baseTime.Format(time.RFC3339Nano),
			wantOk: true,
		},
		{
			name:   "ISO_8601_no_tz",
			input:  baseTime.Format("2006-01-02T15:04:05"),
			wantOk: true,
		},
		{
			name:   "datetime_with_tz",
			input:  baseTime.Format("2006-01-02 15:04:05 -0700"),
			wantOk: true,
		},
		{
			name:   "datetime_with_nanoseconds",
			input:  baseTime.Format("2006-01-02 15:04:05.999999999"),
			wantOk: true,
		},
		{
			name:   "ISO_8601_with_nanoseconds",
			input:  baseTime.Format("2006-01-02T15:04:05.999999999"),
			wantOk: true,
		},
		{
			name:   "datetime_with_nanoseconds_and_tz",
			input:  baseTime.Format("2006-01-02 15:04:05.9999999 -0700 MST"),
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			jsonStr := `"` + tt.input + `"`
			var ft Time
			err := json.Unmarshal([]byte(jsonStr), &ft)

			if tt.wantOk {
				assert.NoError(t, err)
				assert.Equal(t, baseTime.Year(), ft.Year())
				assert.Equal(t, baseTime.Month(), ft.Month())
				assert.Equal(t, baseTime.Day(), ft.Day())
			} else {
				assert.Error(t, err)
			}
		})
	}
}
