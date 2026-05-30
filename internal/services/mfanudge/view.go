package mfanudge

import "time"

// View is the JSON-serialisable projection of a Recommendation returned to the
// frontend by the login, profile and snooze endpoints. A Recommendation that
// does not require a nudge (feature off, non-admin, or 2FA already enrolled)
// maps to a nil *View so callers can omit it from their response.
type View struct {
	Required      bool       `json:"required"`
	ShowNow       bool       `json:"show_now"`
	HardFail      bool       `json:"hard_fail"`
	DaysRemaining int        `json:"days_remaining"`
	HardFailAt    *time.Time `json:"hard_fail_at,omitempty"`
}

// NewView projects a Recommendation into a *View, or nil when no nudge applies.
func NewView(rec Recommendation) *View {
	if !rec.NudgeRequired {
		return nil
	}

	view := &View{
		Required:      rec.NudgeRequired,
		ShowNow:       rec.ShowNow,
		HardFail:      rec.HardFailActive,
		DaysRemaining: rec.DaysRemaining,
	}

	if !rec.HardFailAt.IsZero() {
		hardFailAt := rec.HardFailAt
		view.HardFailAt = &hardFailAt
	}

	return view
}
