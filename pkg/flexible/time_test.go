package flexible

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFlexibleTime_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		wantErr  bool
		validate func(*testing.T, time.Time)
	}{
		{
			name:    "RFC3339 format",
			jsonStr: `"2025-10-20T20:28:10Z"`,
			wantErr: false,
			validate: func(t *testing.T, tt time.Time) {
				t.Helper()

				if tt.Year() != 2025 || tt.Month() != 10 || tt.Day() != 20 {
					t.Errorf("Expected 2025-10-20, got %v", tt)
				}
			},
		},
		{
			name:    "MySQL datetime format",
			jsonStr: `"2025-10-20 20:28:10"`,
			wantErr: false,
			validate: func(t *testing.T, tt time.Time) {
				t.Helper()

				if tt.Year() != 2025 || tt.Month() != 10 || tt.Day() != 20 {
					t.Errorf("Expected 2025-10-20, got %v", tt)
				}
				if tt.Hour() != 20 || tt.Minute() != 28 || tt.Second() != 10 {
					t.Errorf("Expected 20:28:10, got %02d:%02d:%02d", tt.Hour(), tt.Minute(), tt.Second())
				}
			},
		},
		{
			name:    "ISO 8601 without timezone",
			jsonStr: `"2025-10-20T20:28:10"`,
			wantErr: false,
			validate: func(t *testing.T, tt time.Time) {
				t.Helper()

				if tt.Year() != 2025 || tt.Month() != 10 || tt.Day() != 20 {
					t.Errorf("Expected 2025-10-20, got %v", tt)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ft Time
			err := json.Unmarshal([]byte(tt.jsonStr), &ft)

			if (err != nil) != tt.wantErr {
				t.Errorf("Time.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, ft.Time)
			}
		})
	}
}

func TestFlexibleTime_MarshalJSON(t *testing.T) {
	ft := Time{Time: time.Date(2025, 10, 20, 20, 28, 10, 0, time.UTC)}
	data, err := json.Marshal(ft)
	if err != nil {
		t.Errorf("Time.MarshalJSON() error = %v", err)

		return
	}

	expected := `"2025-10-20T20:28:10Z"`
	if string(data) != expected {
		t.Errorf("Time.MarshalJSON() = %v, want %v", string(data), expected)
	}
}
