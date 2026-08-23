package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// StrongETag returns the quoted hex SHA-256 of body, suitable for the ETag
// header and for comparison with If-None-Match.
func StrongETag(body []byte) string {
	sum := sha256.Sum256(body)

	return `"` + hex.EncodeToString(sum[:]) + `"`
}

// IfNoneMatchSatisfied reports whether the request's If-None-Match header
// names etag, so the handler may answer 304 Not Modified. The header is a
// comma-separated list or "*"; a weak validator prefix (W/) is tolerated,
// which is the weak comparison RFC 9110 §13.1.2 prescribes for GET.
func IfNoneMatchSatisfied(r *http.Request, etag string) bool {
	header := strings.TrimSpace(r.Header.Get("If-None-Match"))
	if header == "" {
		return false
	}

	if header == "*" {
		return true
	}

	want := strings.TrimPrefix(etag, "W/")

	for candidate := range strings.SplitSeq(header, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == want {
			return true
		}
	}

	return false
}
