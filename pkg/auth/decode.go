package auth

import (
	"bytes"
	"encoding/ascii85"
	"encoding/base64"
)

func DecodeWithPrefix(s []byte) []byte {
	if after, ok := bytes.CutPrefix(s, []byte("base64:")); ok {
		encoded := after
		decoded := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
		n, err := base64.StdEncoding.Decode(decoded, encoded)
		if err != nil {
			return s
		}

		return decoded[:n]
	}

	if after, ok := bytes.CutPrefix(s, []byte("ascii85:")); ok {
		encoded := after
		decoded := make([]byte, len(encoded))
		n, _, err := ascii85.Decode(decoded, encoded, true)
		if err != nil {
			return s
		}

		return decoded[:n]
	}

	return s
}
