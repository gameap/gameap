// API Security Tests for OWASP API Security Top 10:2023.
// Category: API2:2023 — Broken Authentication / API8:2023 — Security
// Misconfiguration.
//
// Pins the soft-nudge + hard-fail escalation logic (ASVS §4.3.1) the
// login flow consults to decide whether an admin without 2FA gets a
// session, a recommendation, or a forced enrollment challenge.
package mfanudge_test

import (
	"testing"
	"time"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/services/mfanudge"
	"github.com/stretchr/testify/assert"
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func cfgWithMFA(require bool, hardFailDays int) config.Config {
	var cfg config.Config
	cfg.Auth.RequireMFAForAdmins = require
	cfg.Auth.MFAHardFailDays = hardFailDays

	return cfg
}

func TestCompute_NotEnabledForNonAdmins(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := mfanudge.New(cfgWithMFA(true, 30), fixedClock(now))

	rec := svc.Compute(mfanudge.Snapshot{IsAdmin: false})

	assert.False(t, rec.NudgeRequired)
	assert.False(t, rec.ShowNow)
}

func TestCompute_NotEnabledWhenUserHas2FA(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := mfanudge.New(cfgWithMFA(true, 30), fixedClock(now))

	rec := svc.Compute(mfanudge.Snapshot{IsAdmin: true, TwoFactorEnabled: true})

	assert.False(t, rec.NudgeRequired)
}

func TestCompute_NotEnabledWhenOperatorFlagOff(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := mfanudge.New(cfgWithMFA(false, 30), fixedClock(now))

	rec := svc.Compute(mfanudge.Snapshot{IsAdmin: true, TwoFactorEnabled: false})

	assert.False(t, rec.NudgeRequired,
		"operator-level switch must short-circuit the nudge")
}

func TestCompute_FirstContactRecordsTimestamp(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := mfanudge.New(cfgWithMFA(true, 30), fixedClock(now))

	rec := svc.Compute(mfanudge.Snapshot{IsAdmin: true})

	assert.True(t, rec.NudgeRequired)
	assert.True(t, rec.ShowNow,
		"first contact has no snooze, modal must display immediately")
	assert.True(t, rec.PersistFirstShown,
		"caller must persist FirstShownAt — service signals via flag")
	assert.Equal(t, now, rec.FirstShownAt)
	assert.Equal(t, 30, rec.DaysRemaining)
	assert.False(t, rec.HardFailActive)
}

func TestCompute_SnoozedSuppressesModal(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	shown := now.Add(-2 * 24 * time.Hour) // 2 days ago
	snooze := now.Add(20 * time.Hour)     // 20 h in the future

	svc := mfanudge.New(cfgWithMFA(true, 30), fixedClock(now))

	rec := svc.Compute(mfanudge.Snapshot{
		IsAdmin:      true,
		FirstShownAt: &shown,
		SnoozedUntil: &snooze,
	})

	assert.True(t, rec.NudgeRequired)
	assert.False(t, rec.ShowNow, "snooze in the future must suppress the modal")
	assert.False(t, rec.PersistFirstShown, "first_shown_at already stored — no rewrite")
}

func TestCompute_SnoozeExpiredRevealsModal(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	shown := now.Add(-2 * 24 * time.Hour)
	snooze := now.Add(-1 * time.Hour) // already past

	svc := mfanudge.New(cfgWithMFA(true, 30), fixedClock(now))

	rec := svc.Compute(mfanudge.Snapshot{
		IsAdmin:      true,
		FirstShownAt: &shown,
		SnoozedUntil: &snooze,
	})

	assert.True(t, rec.ShowNow, "expired snooze re-shows the modal")
}

func TestCompute_HardFailOnGracePeriodBoundary(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	shown := now.Add(-30 * 24 * time.Hour) // exactly 30 days ago

	svc := mfanudge.New(cfgWithMFA(true, 30), fixedClock(now))

	rec := svc.Compute(mfanudge.Snapshot{
		IsAdmin:      true,
		FirstShownAt: &shown,
	})

	assert.True(t, rec.HardFailActive,
		"30 days from first_shown_at must engage hard fail")
	assert.Equal(t, 0, rec.DaysRemaining)
	assert.True(t, rec.ShowNow, "hard-fail must always show — no snooze rescue")
}

func TestCompute_HardFailOverridesSnooze(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	shown := now.Add(-31 * 24 * time.Hour) // past grace period
	snooze := now.Add(48 * time.Hour)      // user just snoozed 24h

	svc := mfanudge.New(cfgWithMFA(true, 30), fixedClock(now))

	rec := svc.Compute(mfanudge.Snapshot{
		IsAdmin:      true,
		FirstShownAt: &shown,
		SnoozedUntil: &snooze,
	})

	assert.True(t, rec.HardFailActive)
	assert.True(t, rec.ShowNow, "even a fresh snooze does not bypass hard fail")
}

func TestCompute_HardFailDaysZeroDisablesEscalation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	shown := now.Add(-365 * 24 * time.Hour) // a year ago

	svc := mfanudge.New(cfgWithMFA(true, 0), fixedClock(now))

	rec := svc.Compute(mfanudge.Snapshot{
		IsAdmin:      true,
		FirstShownAt: &shown,
	})

	assert.True(t, rec.NudgeRequired)
	assert.False(t, rec.HardFailActive,
		"AUTH_MFA_HARD_FAIL_DAYS=0 keeps the nudge as pure persuasion")
}

func TestCompute_DaysRemainingRoundsUp(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	shown := now.Add(-7*24*time.Hour - 3*time.Hour) // 7 days + 3h ago

	svc := mfanudge.New(cfgWithMFA(true, 30), fixedClock(now))

	rec := svc.Compute(mfanudge.Snapshot{
		IsAdmin:      true,
		FirstShownAt: &shown,
	})

	// 30 - 7d3h ≈ 22d 21h → round up to 23.
	assert.Equal(t, 23, rec.DaysRemaining,
		"days_remaining must round up so the frontend never shows '0 days' before hard-fail")
}

func TestSnoozeDuration_Is24Hours(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 24*time.Hour, mfanudge.SnoozeDuration,
		"snooze window is documented as 24h — change must update ASVS_L2 doc in lockstep")
}
