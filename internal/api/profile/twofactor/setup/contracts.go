package setup

// twoFactorManager is the slice of the two-factor manager this handler needs:
// produce a fresh secret + otpauth URI and encrypt the secret for storage.
type twoFactorManager interface {
	GenerateSecret(account string) (secret, otpauthURI string, err error)
	EncryptSecret(secret string) (string, error)
}
