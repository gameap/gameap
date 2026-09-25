package idempotency

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"net/http"
)

// hasher turns the client key and the request into keyed hashes, so neither
// the key (it may be derived from customer data) nor the body (it may carry
// a password) is stored, and the stored fingerprint cannot be brute-forced
// without the panel secret.
type hasher struct {
	secret []byte
}

func newHasher(secret []byte) hasher {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("gameap/idempotency"))

	return hasher{secret: mac.Sum(nil)}
}

func (h hasher) keyHash(key string) string {
	mac := hmac.New(sha256.New, h.secret)
	writeField(mac, []byte("key"))
	writeField(mac, []byte(key))

	return hex.EncodeToString(mac.Sum(nil))
}

// fingerprint preserves body bytes because decoding JSON into generic maps
// does not preserve the semantics of the handlers' typed decoders.
func (h hasher) fingerprint(r *http.Request, body []byte) string {
	mac := hmac.New(sha256.New, h.secret)
	writeField(mac, []byte("request"))
	writeField(mac, []byte(r.Method))
	writeField(mac, []byte(r.URL.EscapedPath()))
	writeField(mac, []byte(r.URL.Query().Encode()))

	writeField(mac, []byte("raw"))
	writeField(mac, body)

	return hex.EncodeToString(mac.Sum(nil))
}

// writeField length-prefixes every field, so no two different requests feed
// the same byte stream to the MAC.
func writeField(mac hash.Hash, field []byte) {
	var length [8]byte

	binary.BigEndian.PutUint64(length[:], uint64(len(field)))
	mac.Write(length[:])
	mac.Write(field)
}
