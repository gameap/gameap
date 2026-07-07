package domain

import "time"

// metaKeyPasswordChangedAt records the moment the user's password last changed.
// Stored in the generic User.Metadata bag (RFC3339 string) alongside the MFA
// state; access only through the typed accessors below.
const metaKeyPasswordChangedAt = "password_changed_at"

// PasswordChangedAt returns the moment the user's password was last changed, or
// nil if it has never been recorded (e.g. accounts predating this tracking).
//
// The auth layer rejects any session token issued — and any personal access
// token created — before this timestamp, so changing a password invalidates all
// pre-existing credentials.
func (u *User) PasswordChangedAt() *time.Time {
	return u.metadataTime(metaKeyPasswordChangedAt)
}

// SetPasswordChangedAt records the password-change time. Call it whenever the
// password hash is replaced (self-service change, admin reset).
func (u *User) SetPasswordChangedAt(t *time.Time) {
	u.setMetadataTime(metaKeyPasswordChangedAt, t)
}
