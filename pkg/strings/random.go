package strings

import (
	"crypto/rand"
	"math/big"

	"github.com/pkg/errors"
)

const (
	characterSet = "abcdedfghijklmnopqrstABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

func CryptoRandomString(length int) (string, error) {
	password := make([]byte, 0, length)
	m := big.NewInt(int64(len(characterSet)))
	for range length {
		n, err := rand.Int(rand.Reader, m)
		if err != nil {
			return "", errors.WithMessage(err, "failed to generate random number")
		}
		character := characterSet[n.Int64()]
		password = append(password, character)
	}

	return string(password), nil
}
