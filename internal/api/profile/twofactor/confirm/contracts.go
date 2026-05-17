package confirm

// twoFactorManager is the slice of the two-factor manager this handler needs:
// validate the first TOTP code against the pending secret and mint the
// initial recovery-code set.
type twoFactorManager interface {
	ValidateTOTP(encryptedSecret, code string, lastStep *int64) (ok bool, usedStep int64, err error)
	GenerateRecoveryCodes() (plain []string, encoded string, err error)
}
