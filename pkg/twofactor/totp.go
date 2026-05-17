package twofactor

import (
	"crypto/subtle"
	"time"

	"github.com/pkg/errors"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// TOTP parameters. SHA1 / 6 digits / 30s is the only combination every common
// authenticator app (Google Authenticator, Authy, 1Password, …) supports
// reliably, so it is fixed rather than configurable.
const (
	totpPeriod uint = 30
	totpSkew   uint = 1
	totpDigits      = otp.DigitsSix
	totpAlgo        = otp.AlgorithmSHA1
)

// GenerateSecret creates a new TOTP secret and its otpauth:// provisioning
// URI. The secret is returned in plaintext for the one-time enrollment display
// only; the caller must encrypt it before persisting.
func (m *Manager) GenerateSecret(account string) (secret, otpauthURI string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      m.issuer,
		AccountName: account,
		Period:      totpPeriod,
		Digits:      totpDigits,
		Algorithm:   totpAlgo,
	})
	if err != nil {
		return "", "", errors.Wrap(err, "failed to generate totp secret")
	}

	return key.Secret(), key.URL(), nil
}

func validateOpts() totp.ValidateOpts {
	return totp.ValidateOpts{
		Period:    totpPeriod,
		Skew:      totpSkew,
		Digits:    totpDigits,
		Algorithm: totpAlgo,
	}
}

// ValidateTOTP decrypts the stored secret and checks code within the skew
// window. Any time-step at or below lastStep is rejected, so a code observed
// by an attacker cannot be replayed within its validity window. On success the
// matched step is returned for the caller to persist as the new lastStep.
func (m *Manager) ValidateTOTP(encryptedSecret, code string, lastStep *int64) (ok bool, usedStep int64, err error) {
	secret, err := m.cipher.Decrypt(encryptedSecret)
	if err != nil {
		return false, 0, errors.WithMessage(err, "failed to decrypt totp secret")
	}

	now := m.now()
	currentStep := now.Unix() / int64(totpPeriod)
	opts := validateOpts()
	skew := int64(totpSkew)

	for delta := -skew; delta <= skew; delta++ {
		step := currentStep + delta
		if step < 0 {
			continue
		}
		if lastStep != nil && step <= *lastStep {
			continue
		}

		expected, genErr := totp.GenerateCodeCustom(secret, time.Unix(step*int64(totpPeriod), 0), opts)
		if genErr != nil {
			return false, 0, errors.Wrap(genErr, "failed to generate expected totp code")
		}

		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return true, step, nil
		}
	}

	return false, 0, nil
}
