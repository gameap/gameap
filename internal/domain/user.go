package domain

import "time"

type User struct {
	ID            uint       `db:"id"`             //
	Login         string     `db:"login"`          // maxlen=255
	Email         string     `db:"email"`          // email,maxlen=255
	Password      string     `db:"password"`       //
	RememberToken *string    `db:"remember_token"` //
	Name          *string    `db:"name"`           // maxlen=255
	CreatedAt     *time.Time `db:"created_at"`     //
	UpdatedAt     *time.Time `db:"updated_at"`     //

	TwoFactorEnabled       bool    `db:"two_factor_enabled"`        //
	TwoFactorSecret        *string `db:"two_factor_secret"`         // AES-GCM ciphertext, base64
	TwoFactorRecoveryCodes *string `db:"two_factor_recovery_codes"` // JSON array of hashed one-time codes
	TwoFactorLastUsedStep  *int64  `db:"two_factor_last_used_step"` // last consumed TOTP time-step (replay guard)
}
