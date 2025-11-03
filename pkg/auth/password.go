package auth

import (
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

const (
	DefaultBcryptCost = bcrypt.DefaultCost
)

// HashPassword hashes a plain text password using bcrypt.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), DefaultBcryptCost)
	if err != nil {
		return "", errors.Wrap(err, "failed to hash password")
	}

	return string(bytes), nil
}

// VerifyPassword compares a bcrypt hashed password with its possible plaintext equivalent.
func VerifyPassword(hashedPassword, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	if err != nil {
		return errors.Wrap(err, "password verification failed")
	}

	return nil
}
