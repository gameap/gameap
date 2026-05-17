// Package filemanagerhttp centralizes the response headers used when serving
// user-controlled files from a server sandbox. User files must never run as
// active content in the panel's (credentialed) origin, so inline rendering is
// restricted to a small safe allowlist and everything else is forced to
// download. See security review finding #5.
package filemanagerhttp

import (
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// SafeContentHeaders sets Content-Type, Content-Disposition,
// X-Content-Type-Options and Content-Security-Policy so a stored HTML/SVG/JS
// file cannot execute in the panel origin and steal another user's session.
//
// daemonMime is the content type reported by the daemon/detector. It is only
// honored for an allowlist of inert types (images except SVG, text/plain,
// application/pdf); anything else is served as an opaque attachment.
func SafeContentHeaders(h http.Header, filename, daemonMime string) {
	disposition := "attachment"
	contentType := "application/octet-stream"

	if inlineAllowed(daemonMime) {
		disposition = "inline"
		contentType = daemonMime
	}

	h.Set("Content-Type", contentType)
	h.Set("Content-Disposition", contentDisposition(disposition, filename))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox")
}

func inlineAllowed(m string) bool {
	m = strings.ToLower(strings.TrimSpace(m))
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}

	switch {
	case m == "image/svg+xml":
		// SVG can embed scripts — never inline.
		return false
	case strings.HasPrefix(m, "image/"):
		return true
	case m == "text/plain":
		return true
	case m == "application/pdf":
		return true
	default:
		return false
	}
}

// contentDisposition builds an RFC 2231 / 6266 compliant header value so a
// filename containing quotes, backslashes or non-ASCII cannot break the header
// or inject parameters.
func contentDisposition(disposition, filename string) string {
	asciiSafe := stripNonASCII(filename)

	return mime.FormatMediaType(disposition, map[string]string{
		"filename":  asciiSafe,
		"filename*": "UTF-8''" + url.PathEscape(filename),
	})
}

func stripNonASCII(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x80 && r != '"' && r != '\\' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}

	out := b.String()
	if out == "" {
		return "download"
	}

	return out
}
