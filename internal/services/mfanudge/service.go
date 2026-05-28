// Package mfanudge computes the MFA-nudge recommendation for admin
// accounts and gates the hard-fail escalation. The package is
// intentionally pure-logic: it consumes a user record + clock + config
// and returns a Recommendation. Persistence (writing first_shown_at /
// snoozed_until to the users table) is the caller's responsibility so
// the handler tier owns transactions.
//
// ASVS 4.3.1 (admin-MFA enforcement). When operators set
// AUTH_REQUIRE_MFA_FOR_ADMINS=true and AUTH_MFA_HARD_FAIL_DAYS>0 the
// requirement turns into "Met" — the soft nudge persuades admins to
// enrol within the grace window, the hard fail closes the loophole
// after the window expires.
package mfanudge

import (
	"time"

	"github.com/gameap/gameap/internal/config"
)

// Snapshot is the per-user nudge state read from the database. Both
// timestamps are optional: a brand-new admin without any prior nudge
// has both nil, which the service treats as "first contact".
type Snapshot struct {
	TwoFactorEnabled bool
	IsAdmin          bool
	FirstShownAt     *time.Time
	SnoozedUntil     *time.Time
}

// Recommendation is the answer to "what should the API tell this user
// about MFA right now?". It is intentionally a value type — the handler
// either marshals it into a response or feeds it back into the snooze
// endpoint, with no further state on the service.
type Recommendation struct {
	// NudgeRequired is true when the user is an admin without 2FA and
	// the operator has set AUTH_REQUIRE_MFA_FOR_ADMINS=true. Non-admins
	// and users who already have 2FA enabled get a zero-valued
	// Recommendation that the handler can omit from the response.
	NudgeRequired bool

	// ShowNow tells the frontend whether to display the modal right
	// now. False during the snooze window; true once the snooze has
	// expired or was never set.
	ShowNow bool

	// HardFailActive is true once the grace period has elapsed. The
	// login flow uses this to issue an enrollment challenge instead
	// of a session token.
	HardFailActive bool

	// FirstShownAt is the timestamp the very first nudge was recorded
	// (or, if the snapshot did not carry one yet, the time the
	// service is about to record one). Returned so the handler can
	// persist it on the user row.
	FirstShownAt time.Time

	// HardFailAt is the moment hard-fail engages. Frontend can use
	// this to drive a "N days remaining" countdown.
	HardFailAt time.Time

	// DaysRemaining is the integer days until hard-fail. 0 once the
	// hard-fail threshold has been crossed.
	DaysRemaining int

	// PersistFirstShown is true when the caller MUST write FirstShownAt
	// into the user row (the snapshot did not have one yet). Avoids
	// repeated writes once the nudge has been seen for the first time.
	PersistFirstShown bool
}

// Service computes the Recommendation. It carries the config + clock so
// tests can inject a fake clock to exercise the grace-period boundary
// without time.Sleep.
type Service struct {
	cfg   config.Config
	clock func() time.Time
}

// New wires the service. nowFn defaults to time.Now when nil.
func New(cfg config.Config, nowFn func() time.Time) *Service {
	if nowFn == nil {
		nowFn = time.Now
	}

	return &Service{cfg: cfg, clock: nowFn}
}

// Compute returns the Recommendation for a Snapshot. A user who is not an
// admin, or whose 2FA is already enabled, or whose operator has not
// enabled RequireMFAForAdmins, gets a zero-valued result so the caller
// can short-circuit cheaply.
func (s *Service) Compute(snap Snapshot) Recommendation {
	if !s.cfg.Auth.RequireMFAForAdmins {
		return Recommendation{}
	}

	if !snap.IsAdmin {
		return Recommendation{}
	}

	if snap.TwoFactorEnabled {
		return Recommendation{}
	}

	now := s.clock()

	rec := Recommendation{
		NudgeRequired: true,
	}

	if snap.FirstShownAt == nil {
		rec.FirstShownAt = now
		rec.PersistFirstShown = true
	} else {
		rec.FirstShownAt = *snap.FirstShownAt
	}

	if s.cfg.Auth.MFAHardFailDays > 0 {
		rec.HardFailAt = rec.FirstShownAt.Add(time.Duration(s.cfg.Auth.MFAHardFailDays) * 24 * time.Hour)

		if remaining := rec.HardFailAt.Sub(now); remaining <= 0 {
			rec.HardFailActive = true
			rec.DaysRemaining = 0
		} else {
			// Round UP so a partial day still shows a positive number.
			rec.DaysRemaining = int((remaining + 24*time.Hour - 1) / (24 * time.Hour))
		}
	}

	// ShowNow gating: a snooze in the future suppresses the modal.
	// Hard-fail overrides snooze — once the grace window is gone, the
	// modal becomes mandatory.
	rec.ShowNow = rec.HardFailActive ||
		snap.SnoozedUntil == nil ||
		!snap.SnoozedUntil.After(now)

	return rec
}

// SnoozeDuration is the fixed window applied when the user dismisses
// the modal. Kept as a const because the user-experience design pinned
// 24h as the snooze period; if you change it, change docs/security/
// ASVS_L2.md C-9 residual at the same time.
const SnoozeDuration = 24 * time.Hour
