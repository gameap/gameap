package idempotency

import (
	"net/http"
	"strings"

	"github.com/pkg/errors"
)

const (
	// HeaderKey carries the client-chosen key of a request.
	HeaderKey = "Idempotency-Key"

	// HeaderReplayed marks a response replayed from a stored outcome.
	HeaderReplayed = "Idempotent-Replayed"

	maxKeyLength = 255
)

// ParseKey reads the Idempotency-Key header. It accepts the Structured Field
// string of the IETF draft ("..." with \" and \\ escapes) and the bare token
// most clients send. ok is false when the header is absent or empty: an empty
// key identifies nothing, and a client that sends one by accident keeps the
// behaviour it had before the header was supported.
func ParseKey(header http.Header) (key string, ok bool, err error) {
	values := header.Values(HeaderKey)

	switch len(values) {
	case 0:
		return "", false, nil
	case 1:
	default:
		return "", false, errors.WithMessage(ErrInvalidKey, "the header must be sent once")
	}

	key = strings.TrimSpace(values[0])

	if strings.HasPrefix(key, `"`) {
		key, err = unquoteString(key)
		if err != nil {
			return "", false, err
		}
	}

	if key == "" {
		return "", false, nil
	}

	if len(key) > maxKeyLength {
		return "", false, errors.WithMessage(ErrInvalidKey, "the key must be at most 255 characters long")
	}

	for i := range len(key) {
		if key[i] < 0x20 || key[i] > 0x7e {
			return "", false, errors.WithMessage(ErrInvalidKey, "the key must contain printable ASCII characters only")
		}
	}

	return key, true, nil
}

// unquoteString decodes an RFC 8941 sf-string: DQUOTE *( unescaped / "\" (
// DQUOTE / "\" ) ) DQUOTE.
func unquoteString(quoted string) (string, error) {
	if len(quoted) < 2 || quoted[len(quoted)-1] != '"' {
		return "", errors.WithMessage(ErrInvalidKey, "the quoted key is not terminated")
	}

	var b strings.Builder

	for i := 1; i < len(quoted)-1; i++ {
		switch c := quoted[i]; c {
		case '\\':
			i++
			if i == len(quoted)-1 || (quoted[i] != '"' && quoted[i] != '\\') {
				return "", errors.WithMessage(ErrInvalidKey, "the quoted key has an invalid escape")
			}

			b.WriteByte(quoted[i])
		case '"':
			return "", errors.WithMessage(ErrInvalidKey, "the quoted key has an unescaped quote")
		default:
			b.WriteByte(c)
		}
	}

	return b.String(), nil
}
