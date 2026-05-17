package disable

// twoFactorManager is the slice of the two-factor manager this handler needs:
// verify the supplied code (TOTP or a recovery code) before tearing 2FA down.
type twoFactorManager interface {
	ValidateTOTP(encryptedSecret, code string, lastStep *int64) (ok bool, usedStep int64, err error)
	ConsumeRecoveryCode(encoded, input string) (updated string, ok bool, err error)
}
