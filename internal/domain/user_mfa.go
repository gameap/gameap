package domain

import "time"

// MFA-nudge state is kept inside the generic User.Metadata bag (rather than
// dedicated columns) under these keys, stored as RFC3339 strings so they
// round-trip cleanly through JSON/JSONB. Access only through the typed
// accessors below so the key names never leak into handlers.
const (
	metaKeyMFAFirstShownAt = "mfa_first_shown_at"
	metaKeyMFASnoozedUntil = "mfa_snoozed_until"
)

// MFAFirstShownAt returns the moment the first admin-MFA nudge was recorded for
// this user, or nil when none has been shown yet.
func (u *User) MFAFirstShownAt() *time.Time {
	return u.metadataTime(metaKeyMFAFirstShownAt)
}

// SetMFAFirstShownAt records the first-nudge time, or clears it when t is nil.
func (u *User) SetMFAFirstShownAt(t *time.Time) {
	u.setMetadataTime(metaKeyMFAFirstShownAt, t)
}

// MFASnoozedUntil returns the moment until which the MFA-nudge modal is
// suppressed, or nil when the user has never snoozed it.
func (u *User) MFASnoozedUntil() *time.Time {
	return u.metadataTime(metaKeyMFASnoozedUntil)
}

// SetMFASnoozedUntil records the snooze deadline, or clears it when t is nil.
func (u *User) SetMFASnoozedUntil(t *time.Time) {
	u.setMetadataTime(metaKeyMFASnoozedUntil, t)
}

func (u *User) metadataTime(key string) *time.Time {
	if u.Metadata == nil {
		return nil
	}

	raw, ok := u.Metadata[key]
	if !ok {
		return nil
	}

	str, ok := raw.(string)
	if !ok {
		return nil
	}

	parsed, err := time.Parse(time.RFC3339, str)
	if err != nil {
		return nil
	}

	return &parsed
}

func (u *User) setMetadataTime(key string, t *time.Time) {
	if t == nil {
		delete(u.Metadata, key)

		return
	}

	if u.Metadata == nil {
		u.Metadata = Metadata{}
	}

	u.Metadata[key] = t.UTC().Format(time.RFC3339)
}
