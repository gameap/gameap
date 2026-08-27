package ssomint

import (
	"net"
	"strings"
	"unicode"

	"github.com/gameap/gameap/pkg/api"
)

var (
	errUserIDRequired  = api.NewValidationError("user_id is required")
	errInvalidRedirect = api.NewValidationError(
		"redirect_to must be a relative path within the panel",
	)
	errInvalidClientIP = api.NewValidationError("client_ip must be a valid IP address")
)

const maxRedirectLength = 512

const maxBodySize = 1024

type ticketInput struct {
	UserID uint `json:"user_id"`

	// RedirectTo is where the panel sends the browser after the ticket is
	// redeemed. Validated here and then carried inside the cached payload, so
	// the exchange endpoint never reads a redirect target from the browser.
	RedirectTo string `json:"redirect_to,omitempty"`

	// ClientIP optionally binds the ticket to the address that will redeem it.
	ClientIP string `json:"client_ip,omitempty"`
}

func (in *ticketInput) Validate() error {
	if in.UserID == 0 {
		return errUserIDRequired
	}

	if !isSafeRedirect(in.RedirectTo) {
		return errInvalidRedirect
	}

	// The exchange handler compares this value verbatim against the redeeming
	// address (read back through audit.ClientIP, which does not canonicalise),
	// so require a literal IP address and store it unchanged. A hostname, a
	// malformed value or a whitespace-padded one would never match and would
	// silently mint a ticket that can never be redeemed.
	if in.ClientIP != "" && net.ParseIP(in.ClientIP) == nil {
		return errInvalidClientIP
	}

	return nil
}

// isSafeRedirect accepts an empty value or a path rooted in the panel itself.
// Anything that could leave the origin is rejected: an absolute URL, a
// protocol-relative "//host" and the "/\host" form browsers also treat as
// protocol-relative.
func isSafeRedirect(redirect string) bool {
	if redirect == "" {
		return true
	}

	if len(redirect) > maxRedirectLength {
		return false
	}

	if !strings.HasPrefix(redirect, "/") {
		return false
	}

	if strings.HasPrefix(redirect, "//") || strings.HasPrefix(redirect, `/\`) {
		return false
	}

	return isPrintableASCII(redirect)
}

func isPrintableASCII(value string) bool {
	for _, r := range value {
		if r > unicode.MaxASCII || !unicode.IsPrint(r) {
			return false
		}
	}

	return true
}
