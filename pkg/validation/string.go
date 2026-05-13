package validation

import (
	"regexp"
	"slices"
	"strings"
)

// IsAlphanumeric checks if a string contains only lowercase letters (a-z) and digits (0-9).
// Returns true if the string is alphanumeric, false otherwise.
// Empty strings return false.
func IsAlphanumeric(s string) bool {
	if len(s) == 0 {
		return false
	}

	for _, ch := range s {
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') {
			return false
		}
	}

	return true
}

// IsAlphanumericMixed checks if a string contains only letters (a-z, A-Z) and digits (0-9).
// Returns true if the string is alphanumeric, false otherwise.
// Empty strings return false.
func IsAlphanumericMixed(s string) bool {
	if len(s) == 0 {
		return false
	}

	for _, ch := range s {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') {
			return false
		}
	}

	return true
}

// IsSlug checks if a string contains only lowercase letters (a-z), digits (0-9), underscores (_), and hyphens (-).
// Returns true if the string is a valid slug, false otherwise.
// Empty strings return false.
func IsSlug(s string) bool {
	if len(s) == 0 {
		return false
	}

	for _, ch := range s {
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '_' && ch != '-' {
			return false
		}
	}

	return true
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func IsEmail(s string) bool {
	return emailRegex.MatchString(s)
}

// IsRelativeServerPath reports whether s is a safe relative server directory:
// not anchored to a filesystem root, not a Windows drive path, and free of ".."
// segments. Used to reject inputs that would land on the daemon as an absolute
// path and trigger a double-join with the daemon's own work directory.
// Empty strings are not considered valid; callers that allow optional values
// should check emptiness separately.
func IsRelativeServerPath(s string) bool {
	if s == "" {
		return false
	}

	if s[0] == '/' || s[0] == '\\' {
		return false
	}

	if len(s) >= 2 && s[1] == ':' && IsASCIILetter(s[0]) {
		return false
	}

	normalized := strings.ReplaceAll(s, "\\", "/")

	return !slices.Contains(strings.Split(normalized, "/"), "..")
}

// IsASCIILetter reports whether c is an ASCII letter (a-z or A-Z).
func IsASCIILetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}
