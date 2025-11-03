package carbon

import (
	"testing"
	"time"
)

func TestParseInterval(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "simple years",
			input:    "2 years",
			expected: 2 * 365 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "simple months",
			input:    "6 months",
			expected: 6 * 30 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "simple weeks",
			input:    "3 weeks",
			expected: 3 * 7 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "simple days",
			input:    "10 days",
			expected: 10 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "simple hours",
			input:    "24 hours",
			expected: 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "simple minutes",
			input:    "30 minutes",
			expected: 30 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "simple seconds",
			input:    "45 seconds",
			expected: 45 * time.Second,
			wantErr:  false,
		},
		{
			name:     "milliseconds",
			input:    "500 milliseconds",
			expected: 500 * time.Millisecond,
			wantErr:  false,
		},
		{
			name:     "microseconds",
			input:    "1500 microseconds",
			expected: 1500 * time.Microsecond,
			wantErr:  false,
		},
		{
			name:     "combined units",
			input:    "1 year 2 months 3 days 4 hours 5 minutes 6 seconds",
			expected: 365*24*time.Hour + 2*30*24*time.Hour + 3*24*time.Hour + 4*time.Hour + 5*time.Minute + 6*time.Second,
			wantErr:  false,
		},
		{
			name:     "abbreviated units",
			input:    "1y 2mo 3d 4h 5m 6s",
			expected: 365*24*time.Hour + 2*30*24*time.Hour + 3*24*time.Hour + 4*time.Hour + 5*time.Minute + 6*time.Second,
			wantErr:  false,
		},
		{
			name:     "single letter abbreviations",
			input:    "1y 2w 3d 4h 5m 6s",
			expected: 365*24*time.Hour + 2*7*24*time.Hour + 3*24*time.Hour + 4*time.Hour + 5*time.Minute + 6*time.Second,
			wantErr:  false,
		},
		{
			name:     "quarter",
			input:    "1 quarter",
			expected: 3 * 30 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "quarters plural",
			input:    "2 quarters",
			expected: 6 * 30 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "no space between number and unit",
			input:    "1year 2months 3days",
			expected: 365*24*time.Hour + 2*30*24*time.Hour + 3*24*time.Hour,
			wantErr:  false,
		},
		{
			name:     "extra spaces",
			input:    "1   year   2   months",
			expected: 365*24*time.Hour + 2*30*24*time.Hour,
			wantErr:  false,
		},
		{
			name:     "fractional hours",
			input:    "1.5 hours",
			expected: time.Hour + 30*time.Minute,
			wantErr:  false,
		},
		{
			name:     "fractional days",
			input:    "2.5 days",
			expected: 2*24*time.Hour + 12*time.Hour,
			wantErr:  false,
		},
		{
			name:     "fractional weeks",
			input:    "1.5 weeks",
			expected: 7*24*time.Hour + 3*24*time.Hour + 12*time.Hour,
			wantErr:  false,
		},
		{
			name:     "fractional minutes",
			input:    "2.5 minutes",
			expected: 2*time.Minute + 30*time.Second,
			wantErr:  false,
		},
		{
			name:     "fractional seconds",
			input:    "1.5 seconds",
			expected: time.Second + 500*time.Millisecond,
			wantErr:  false,
		},
		{
			name:     "fractional milliseconds",
			input:    "1.5 milliseconds",
			expected: 1500 * time.Microsecond,
			wantErr:  false,
		},
		{
			name:     "complex fractional",
			input:    "1.25 hours",
			expected: time.Hour + 15*time.Minute,
			wantErr:  false,
		},
		{
			name:     "microsecond symbol",
			input:    "100 µs",
			expected: 100 * time.Microsecond,
			wantErr:  false,
		},
		{
			name:     "alternative year abbreviations",
			input:    "1 yr",
			expected: 365 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "alternative year abbreviations plural",
			input:    "2 yrs",
			expected: 2 * 365 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "alternative month abbreviations",
			input:    "3 mo",
			expected: 3 * 30 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "alternative month abbreviations plural",
			input:    "4 mos",
			expected: 4 * 30 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "millisecond abbreviations",
			input:    "100 ms",
			expected: 100 * time.Millisecond,
			wantErr:  false,
		},
		{
			name:     "millisecond alternative",
			input:    "200 milli",
			expected: 200 * time.Millisecond,
			wantErr:  false,
		},
		{
			name:     "microsecond alternative",
			input:    "300 micro",
			expected: 300 * time.Microsecond,
			wantErr:  false,
		},
		{
			name:     "invalid unit",
			input:    "1 invalid",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "millennium is now invalid",
			input:    "1 millennium",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "century is now invalid",
			input:    "1 century",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "decade is now invalid",
			input:    "1 decade",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "invalid number",
			input:    "abc days",
			expected: 0,
			wantErr:  false, // regex won't match, returns empty duration
		},
		{
			name:     "zero values",
			input:    "0 years 0 months 0 days",
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "large numbers",
			input:    "10000 days",
			expected: 10000 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "decimal with many places",
			input:    "1.123456789 hours",
			expected: time.Hour + 7*time.Minute + 24*time.Second + 444440*time.Microsecond,
			wantErr:  false,
		},
		{
			name:     "singular and plural mixed",
			input:    "1 year 2 months 1 day 3 hours",
			expected: 365*24*time.Hour + 2*30*24*time.Hour + 24*time.Hour + 3*time.Hour,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseInterval(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseInterval() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("ParseInterval() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHumanize(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		// Zero duration
		{
			name:     "zero duration",
			duration: 0,
			expected: "0 seconds",
		},
		// Sub-second durations
		{
			name:     "1 microsecond",
			duration: time.Microsecond,
			expected: "1 microsecond",
		},
		{
			name:     "multiple microseconds",
			duration: 500 * time.Microsecond,
			expected: "500 microseconds",
		},
		{
			name:     "1 millisecond",
			duration: time.Millisecond,
			expected: "1 millisecond",
		},
		{
			name:     "multiple milliseconds",
			duration: 250 * time.Millisecond,
			expected: "250 milliseconds",
		},
		// Seconds
		{
			name:     "1 second",
			duration: time.Second,
			expected: "1 second",
		},
		{
			name:     "multiple seconds",
			duration: 30 * time.Second,
			expected: "30 seconds",
		},
		{
			name:     "59 seconds",
			duration: 59 * time.Second,
			expected: "59 seconds",
		},
		// Minutes
		{
			name:     "1 minute",
			duration: time.Minute,
			expected: "1 minute",
		},
		{
			name:     "multiple minutes",
			duration: 5 * time.Minute,
			expected: "5 minutes",
		},
		{
			name:     "59 minutes",
			duration: 59 * time.Minute,
			expected: "59 minutes",
		},
		// Hours
		{
			name:     "1 hour",
			duration: time.Hour,
			expected: "1 hour",
		},
		{
			name:     "multiple hours",
			duration: 2 * time.Hour,
			expected: "2 hours",
		},
		{
			name:     "23 hours",
			duration: 23 * time.Hour,
			expected: "23 hours",
		},
		// Days
		{
			name:     "1 day",
			duration: 24 * time.Hour,
			expected: "1 day",
		},
		{
			name:     "multiple days",
			duration: 3 * 24 * time.Hour,
			expected: "3 days",
		},
		{
			name:     "6 days",
			duration: 6 * 24 * time.Hour,
			expected: "6 days",
		},
		// Weeks
		{
			name:     "1 week",
			duration: 7 * 24 * time.Hour,
			expected: "1 week",
		},
		{
			name:     "2 weeks",
			duration: 14 * 24 * time.Hour,
			expected: "2 weeks",
		},
		{
			name:     "3 weeks",
			duration: 21 * 24 * time.Hour,
			expected: "3 weeks",
		},
		// Months (approximate)
		{
			name:     "1 month",
			duration: 30 * 24 * time.Hour,
			expected: "1 month",
		},
		{
			name:     "2 months",
			duration: 60 * 24 * time.Hour,
			expected: "2 months",
		},
		{
			name:     "11 months",
			duration: 330 * 24 * time.Hour,
			expected: "11 months",
		},
		// Years (approximate)
		{
			name:     "1 year",
			duration: 365 * 24 * time.Hour,
			expected: "1 year",
		},
		{
			name:     "2 years",
			duration: 730 * 24 * time.Hour,
			expected: "2 years",
		},
		{
			name:     "10 years",
			duration: 3650 * 24 * time.Hour,
			expected: "10 years",
		},
		// Edge cases between units
		{
			name:     "1 minute and 30 seconds",
			duration: 90 * time.Second,
			expected: "1 minute",
		},
		{
			name:     "1 hour and 30 minutes",
			duration: 90 * time.Minute,
			expected: "1 hour",
		},
		{
			name:     "1 day and 12 hours",
			duration: 36 * time.Hour,
			expected: "1 day",
		},
		// Negative durations
		{
			name:     "negative 1 second",
			duration: -time.Second,
			expected: "-1 second",
		},
		{
			name:     "negative multiple seconds",
			duration: -30 * time.Second,
			expected: "-30 seconds",
		},
		{
			name:     "negative 1 minute",
			duration: -time.Minute,
			expected: "-1 minute",
		},
		{
			name:     "negative 1 hour",
			duration: -time.Hour,
			expected: "-1 hour",
		},
		{
			name:     "negative 1 day",
			duration: -24 * time.Hour,
			expected: "-1 day",
		},
		{
			name:     "negative 1 week",
			duration: -7 * 24 * time.Hour,
			expected: "-1 week",
		},
		{
			name:     "negative microseconds",
			duration: -500 * time.Microsecond,
			expected: "-500 microseconds",
		},
		{
			name:     "negative milliseconds",
			duration: -250 * time.Millisecond,
			expected: "-250 milliseconds",
		},
		// Complex durations (should use largest appropriate unit)
		{
			name:     "1 year 2 months",
			duration: (365 + 60) * 24 * time.Hour,
			expected: "1 year",
		},
		{
			name:     "2 weeks 3 days",
			duration: (14 + 3) * 24 * time.Hour,
			expected: "2 weeks",
		},
		{
			name:     "25 hours",
			duration: 25 * time.Hour,
			expected: "1 day",
		},
		{
			name:     "61 minutes",
			duration: 61 * time.Minute,
			expected: "1 hour",
		},
		{
			name:     "61 seconds",
			duration: 61 * time.Second,
			expected: "1 minute",
		},
		{
			name:     "1001 milliseconds",
			duration: 1001 * time.Millisecond,
			expected: "1 second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Humanize(tt.duration)
			if got != tt.expected {
				t.Errorf("Humanize(%v) = %q, want %q", tt.duration, got, tt.expected)
			}
		})
	}
}

func TestRoundFloat(t *testing.T) {
	tests := []struct {
		name      string
		val       float64
		precision uint
		expected  float64
	}{
		{
			name:      "round to 0 decimals",
			val:       1.5,
			precision: 0,
			expected:  2.0,
		},
		{
			name:      "round to 2 decimals",
			val:       1.234,
			precision: 2,
			expected:  1.23,
		},
		{
			name:      "round to 6 decimals",
			val:       0.9999999,
			precision: 6,
			expected:  1.0,
		},
		{
			name:      "no rounding needed",
			val:       1.25,
			precision: 2,
			expected:  1.25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundFloat(tt.val, tt.precision)
			if got != tt.expected {
				t.Errorf("roundFloat() = %v, want %v", got, tt.expected)
			}
		})
	}
}
