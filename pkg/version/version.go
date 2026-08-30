// Package version compares GameAP component versions (the panel itself and
// gameap-daemon builds reported by nodes). Version strings arrive in several
// shapes: release tags ("v4.4.2"), bare versions reported by daemons ("4.1.2"),
// and development builds ("development", "dev-a1b2c3") that must never be
// treated as comparable.
package version

import (
	"strings"

	"golang.org/x/mod/semver"
)

// Normalize brings a version string to the canonical "vX.Y.Z" form understood
// by golang.org/x/mod/semver. It returns an empty string when the value is not
// a semantic version, which is how development builds are recognised.
func Normalize(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}

	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}

	// Build metadata does not participate in precedence, drop it so that
	// "v4.4.2+build.5" and "v4.4.2" compare equal.
	if idx := strings.IndexByte(v, '+'); idx >= 0 {
		v = v[:idx]
	}

	if !semver.IsValid(v) {
		return ""
	}

	return v
}

// IsNewer reports whether candidate is a strictly newer version than current.
// An unparsable version on either side yields false: a development build is
// never reported as outdated, and a malformed release tag never triggers an
// update notice.
func IsNewer(current, candidate string) bool {
	normalizedCurrent := Normalize(current)
	normalizedCandidate := Normalize(candidate)

	if normalizedCurrent == "" || normalizedCandidate == "" {
		return false
	}

	return semver.Compare(normalizedCandidate, normalizedCurrent) > 0
}

// IsRelease reports whether v is a valid semantic version without a
// pre-release suffix.
func IsRelease(v string) bool {
	normalized := Normalize(v)

	return normalized != "" && semver.Prerelease(normalized) == ""
}
