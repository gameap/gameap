// API Security Tests for OWASP API Security Top 10:2023.
// Category: API2:2023 — Broken Authentication.
//
// Pins the Recommendation→View projection the login, profile and snooze
// endpoints marshal into their responses (ASVS §4.3.1): a non-required
// recommendation must serialise to a nil block the handler omits, and the
// hard-fail deadline must only be exposed once it is actually set.
package mfanudge_test

import (
	"testing"
	"time"

	"github.com/gameap/gameap/internal/services/mfanudge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewView covers OWASP API2:2023. NewView is a pure projection with no
// dependencies, so the table feeds Recommendation values directly and asserts
// on every field of the resulting *View (or that it is nil).
func TestNewView(t *testing.T) {
	t.Parallel()

	hardFailAt := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		rec  mfanudge.Recommendation
		// want is nil when NewView must return nil (no nudge applies).
		want *mfanudge.View
	}{
		{
			name: "no_nudge_maps_to_nil",
			rec:  mfanudge.Recommendation{},
			want: nil,
		},
		{
			name: "soft_nudge_maps_fields_and_omits_hard_fail_at",
			rec: mfanudge.Recommendation{
				NudgeRequired:  true,
				ShowNow:        true,
				HardFailActive: false,
				DaysRemaining:  23,
				// HardFailAt left zero on purpose: AUTH_MFA_HARD_FAIL_DAYS=0
				// keeps the nudge as pure persuasion with no deadline.
			},
			want: &mfanudge.View{
				Required:      true,
				ShowNow:       true,
				HardFail:      false,
				DaysRemaining: 23,
				HardFailAt:    nil,
			},
		},
		{
			name: "hard_fail_exposes_deadline_instant",
			rec: mfanudge.Recommendation{
				NudgeRequired:  true,
				ShowNow:        true,
				HardFailActive: true,
				DaysRemaining:  0,
				HardFailAt:     hardFailAt,
			},
			want: &mfanudge.View{
				Required:      true,
				ShowNow:       true,
				HardFail:      true,
				DaysRemaining: 0,
				HardFailAt:    &hardFailAt,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE / ACT
			got := mfanudge.NewView(tt.rec)

			// ASSERT
			if tt.want == nil {
				assert.Nil(t, got, "a recommendation that does not require a nudge must map to a nil View")

				return
			}

			require.NotNil(t, got, "a required nudge must map to a non-nil View")
			assert.Equal(t, tt.want.Required, got.Required, "required flag must mirror NudgeRequired")
			assert.Equal(t, tt.want.ShowNow, got.ShowNow, "show_now must mirror ShowNow")
			assert.Equal(t, tt.want.HardFail, got.HardFail, "hard_fail must mirror HardFailActive")
			assert.Equal(t, tt.want.DaysRemaining, got.DaysRemaining, "days_remaining must mirror DaysRemaining")

			if tt.want.HardFailAt == nil {
				assert.Nil(t, got.HardFailAt, "a zero HardFailAt must stay nil so it is omitted from the JSON")
			} else {
				require.NotNil(t, got.HardFailAt, "a non-zero HardFailAt must be projected onto the View")
				assert.True(t, tt.want.HardFailAt.Equal(*got.HardFailAt),
					"hard_fail_at must equal the recommendation's hard-fail instant")
			}
		})
	}
}
