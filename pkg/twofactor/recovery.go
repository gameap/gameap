package twofactor

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

const (
	// RecoveryCodeCount is how many one-time codes are issued per (re)generation.
	RecoveryCodeCount = 10

	recoveryGroupLen = 5
	recoveryGroups   = 2

	// recoveryAlphabet excludes vowels and visually ambiguous characters
	// (0/o, 1/l/i) so codes are safe to read aloud and transcribe.
	recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"
)

// recoveryCode is one persisted, hashed, single-use recovery code.
type recoveryCode struct {
	Hash string `json:"hash"`
	Used bool   `json:"used"`
}

// GenerateRecoveryCodes returns the human-readable codes (shown to the user
// exactly once) and the JSON blob of bcrypt hashes to persist. The plaintext
// codes are never stored.
func (m *Manager) GenerateRecoveryCodes() (plain []string, encoded string, err error) {
	plain = make([]string, 0, RecoveryCodeCount)
	stored := make([]recoveryCode, 0, RecoveryCodeCount)

	for range RecoveryCodeCount {
		raw, genErr := randomFromAlphabet(recoveryGroupLen * recoveryGroups)
		if genErr != nil {
			return nil, "", genErr
		}

		hash, hashErr := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
		if hashErr != nil {
			return nil, "", errors.Wrap(hashErr, "failed to hash recovery code")
		}

		plain = append(plain, raw[:recoveryGroupLen]+"-"+raw[recoveryGroupLen:])
		stored = append(stored, recoveryCode{Hash: string(hash)})
	}

	blob, err := json.Marshal(stored)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to marshal recovery codes")
	}

	return plain, string(blob), nil
}

// ConsumeRecoveryCode checks input against the unused stored codes. On a match
// the code is marked used and the updated JSON blob is returned for the caller
// to persist (single-use guarantee). It tolerates spacing/hyphens/case in input.
func (m *Manager) ConsumeRecoveryCode(encoded, input string) (updated string, ok bool, err error) {
	normalized := normalizeRecoveryInput(input)
	if normalized == "" {
		return encoded, false, nil
	}

	var codes []recoveryCode
	if encoded != "" {
		if unmarshalErr := json.Unmarshal([]byte(encoded), &codes); unmarshalErr != nil {
			return encoded, false, errors.Wrap(unmarshalErr, "failed to unmarshal recovery codes")
		}
	}

	for i := range codes {
		if codes[i].Used {
			continue
		}
		if bcrypt.CompareHashAndPassword([]byte(codes[i].Hash), []byte(normalized)) == nil {
			codes[i].Used = true

			blob, marshalErr := json.Marshal(codes)
			if marshalErr != nil {
				return encoded, false, errors.Wrap(marshalErr, "failed to marshal recovery codes")
			}

			return string(blob), true, nil
		}
	}

	return encoded, false, nil
}

func normalizeRecoveryInput(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}

	return b.String()
}

func randomFromAlphabet(length int) (string, error) {
	out := make([]byte, 0, length)
	alphabetSize := big.NewInt(int64(len(recoveryAlphabet)))

	for range length {
		n, err := rand.Int(rand.Reader, alphabetSize)
		if err != nil {
			return "", errors.Wrap(err, "failed to generate random recovery code")
		}
		out = append(out, recoveryAlphabet[n.Int64()])
	}

	return string(out), nil
}
