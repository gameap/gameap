package validation

import "regexp"

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
