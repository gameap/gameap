package validation

import (
	"net"
	"regexp"
)

var (
	hostnameRegex = regexp.MustCompile(
		`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`,
	)
	ipLikeRegex = regexp.MustCompile(`^[\d.]+$|^[\da-fA-F:]+$`)
)

// IsValidIPOrHostname checks if a string is a valid IP address (IPv4 or IPv6) or a valid hostname.
// It accepts:
// - Valid IPv4 addresses (e.g., "192.168.1.1")
// - Valid IPv6 addresses (e.g., "2001:0db8:85a3::8a2e:0370:7334", "::1")
// - Valid hostnames (e.g., "example.com", "game-server.example.com")
//
// It rejects:
// - Invalid IP addresses that look like IPs but aren't (e.g., "192.168.1.999")
// - Hostnames longer than 253 characters (DNS specification limit)
// - Invalid hostname formats
//
// Returns true if the value is valid, false otherwise.
func IsValidIPOrHostname(value string) bool {
	if net.ParseIP(value) != nil {
		return true
	}

	if ipLikeRegex.MatchString(value) {
		return false
	}

	if len(value) > 253 {
		return false
	}

	return hostnameRegex.MatchString(value)
}
