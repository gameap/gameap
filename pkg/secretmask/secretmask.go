// Package secretmask replaces secret values with a fixed placeholder in text
// that is about to be shown to a user.
//
// It is meant for output the panel does not control the shape of — game server
// console logs, RCON replies — where a secret such as the RCON password can
// appear anywhere inside free-form text and therefore cannot be omitted at the
// response-field level the way node daemon credentials are.
package secretmask

import (
	"bytes"
	"cmp"
	"slices"
	"strings"
)

// Placeholder is what every occurrence of a secret is replaced with.
const Placeholder = "******"

// Masker replaces a fixed set of secrets with Placeholder.
//
// A Masker is immutable after New and safe for concurrent use. The zero value
// and a nil *Masker are valid no-op maskers, so call sites do not need nil
// checks.
type Masker struct {
	secrets []string
}

// New builds a Masker for the given secrets.
//
// Empty values and duplicates are dropped; when nothing is left the returned
// Masker is a no-op and reports Empty. Every other value is masked however
// short it is: a secret that leaks is worse than output a very short secret
// over-matches in.
func New(secrets ...string) *Masker {
	filtered := make([]string, 0, len(secrets))

	for _, secret := range secrets {
		if secret == "" {
			continue
		}

		if !slices.Contains(filtered, secret) {
			filtered = append(filtered, secret)
		}
	}

	// Longest first, so a secret that contains another one is replaced whole rather than
	// being broken up by the shorter match.
	slices.SortStableFunc(filtered, func(a, b string) int {
		return cmp.Compare(len(b), len(a))
	})

	return &Masker{secrets: filtered}
}

// Empty reports whether the Masker has nothing to replace.
//
// Use it to skip installing a masking step at all when no secret is
// configured.
func (m *Masker) Empty() bool {
	return m == nil || len(m.secrets) == 0
}

// String returns s with every occurrence of every secret replaced by
// Placeholder. Input without any match is returned as is.
func (m *Masker) String(s string) string {
	if m.Empty() || s == "" {
		return s
	}

	for _, secret := range m.secrets {
		if !strings.Contains(s, secret) {
			continue
		}

		s = strings.ReplaceAll(s, secret, Placeholder)
	}

	return s
}

// Bytes returns b with every occurrence of every secret replaced by
// Placeholder.
//
// The input slice is never modified. When nothing matches the very same slice
// is returned, so masking a stream of frames that carry no secret costs no
// allocations.
func (m *Masker) Bytes(b []byte) []byte {
	if m.Empty() || len(b) == 0 {
		return b
	}

	for _, secret := range m.secrets {
		secretBytes := []byte(secret)
		if !bytes.Contains(b, secretBytes) {
			continue
		}

		b = bytes.ReplaceAll(b, secretBytes, []byte(Placeholder))
	}

	return b
}
