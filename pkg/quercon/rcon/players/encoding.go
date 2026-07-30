package players

import (
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// decodeLatin1IfInvalidUTF8 returns s unchanged when it is already valid UTF-8 and reinterprets
// it as ISO 8859-1 otherwise.
//
// Game servers put player nicknames on the wire as raw bytes in whatever code page the server
// runs, and JSON encoding would replace invalid sequences with U+FFFD and lose them. Decoding
// unconditionally is not an option either: BattlEye games and modern ioquake3 forks send UTF-8,
// which an unconditional Latin-1 decode would turn into mojibake. For genuinely Latin-1 input
// both forms produce the same result.
func decodeLatin1IfInvalidUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}

	decoded, err := charmap.ISO8859_1.NewDecoder().String(s)
	if err != nil {
		return s
	}

	return decoded
}
