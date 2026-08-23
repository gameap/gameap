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
// names etag, so the handler may answer 304 Not Modified. The header is
// either a comma-separated list of entity-tags (possibly split over several
// field lines) or "*" on its own; a list that mixes the two is malformed and
// matches nothing. A weak validator prefix (W/) is tolerated, which is the
// weak comparison RFC 9110 §13.1.2 prescribes for GET.
func IfNoneMatchSatisfied(r *http.Request, etag string) bool {
	header := strings.TrimSpace(strings.Join(r.Header.Values("If-None-Match"), ","))
	if header == "" {
		return false
	}

	want := strings.TrimPrefix(etag, "W/")
	matched, wildcard, members := false, false, 0

	for candidate := range strings.SplitSeq(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}

		members++

		switch {
		case candidate == "*":
			wildcard = true
		case strings.TrimPrefix(candidate, "W/") == want:
			matched = true
		}
	}

	if wildcard {
		return members == 1
	}

	return matched
}
