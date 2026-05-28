package auth

import (
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// dummyPasswordPlaintext is the secret VerifyDummyPassword tests against.
// The exact value is irrelevant — what matters is that no real user can ever
// have a hash for this string (it is 64 random bytes, far above any human
// password). The constant is kept private so a future audit cannot mistake
// it for a credential.
//
//nolint:gosec // not a credential; used only to seed the timing-oracle dummy hash.
const dummyPasswordPlaintext = "1d4e54e89c2c8a3d6e0c2f3c4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b"

var (
	dummyHashOnce sync.Once
	dummyHash     []byte
)

// VerifyDummyPassword runs a constant-time bcrypt compare against a hash
// computed lazily at the current ActiveBcryptCost, so that login handlers
// can call it for non-existent users and equalise the request latency
// between "user known" and "user unknown" code paths.
//
// The dummy is generated once on first use and cached for the process
// lifetime. Subsequent calls become trivially short (one bcrypt verify) but
// remain matched in wall-clock time to a real VerifyPassword against a hash
// of the same cost, which is the whole point of the helper.
//
// The error return is intentionally discarded by callers: this function
// exists for timing equalisation, not for authenticating the dummy.
func VerifyDummyPassword(candidate string) {
	dummyHashOnce.Do(func() {
		// Best-effort: if bcrypt itself fails (cost out of range, RNG
		// unavailable) leave dummyHash nil. The Verify call below will
		// then short-circuit with bcrypt.ErrHashTooShort, which is still
		// a bcrypt operation that consumes the right amount of wall-clock
		// time relative to the rejection path that found no user.
		hashed, err := HashPasswordWithCost(dummyPasswordPlaintext, ActiveBcryptCost())
		if err != nil {
			return
		}

		dummyHash = []byte(hashed)
	})

	if len(dummyHash) == 0 {
		return
	}

	// We use bcrypt.CompareHashAndPassword directly instead of VerifyPassword
	// because we want every call to actually execute bcrypt's KDF (the
	// VerifyPassword legacy fallback can short-circuit on length and would
	// shave the timing curve).
	_ = bcrypt.CompareHashAndPassword(dummyHash, preHash(candidate))
}
