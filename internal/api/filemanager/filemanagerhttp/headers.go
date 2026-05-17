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
// honored, and only as its bare type with parameters stripped, for an explicit
// allowlist of inert types (common raster images, text/plain, application/pdf);
// anything else is served as an opaque attachment.
func SafeContentHeaders(h http.Header, filename, daemonMime string) {
	disposition := "attachment"
	contentType := "application/octet-stream"

	if bare, ok := inlineMime(daemonMime); ok {
		disposition = "inline"
		contentType = bare
	}

	h.Set("Content-Type", contentType)
	h.Set("Content-Disposition", contentDisposition(disposition, filename))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox")
}

// inlineSafeMimes is an explicit allowlist of inert media types browsers render
// without executing script. image/svg+xml is intentionally absent (SVG can
// embed script); a HasPrefix("image/") check is deliberately avoided so an
// unknown or future image/* subtype a browser treats as active content is
// still forced to download. Everything not listed is an opaque attachment.
var inlineSafeMimes = map[string]struct{}{
	"image/png":                {},
	"image/jpeg":               {},
	"image/gif":                {},
	"image/webp":               {},
	"image/bmp":                {},
	"image/x-icon":             {},
	"image/vnd.microsoft.icon": {},
	"text/plain":               {},
	"application/pdf":          {},
}

// inlineMime returns the bare, lowercased media type (parameters such as
// "; charset=binary" stripped) and whether it is on the inline allowlist. The
// bare form is what is written to Content-Type so a daemon-supplied parameter
// cannot ride along into the response header and the value always matches the
// allowlist check.
func inlineMime(m string) (string, bool) {
	m = strings.ToLower(strings.TrimSpace(m))
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}

	_, ok := inlineSafeMimes[m]

	return m, ok
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

// stripNonASCII keeps only printable ASCII (0x20–0x7e) except '"' and '\\',
// replacing everything else — non-ASCII, C0 control bytes and DEL (notably CR,
// LF, HT) — with '_'. This keeps raw control bytes out of the Content-
// Disposition filename token regardless of how the downstream mime encoder
// handles them; the exact bytes still travel safely in the percent-encoded
// filename* parameter.
func stripNonASCII(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r < 0x7f && r != '"' && r != '\\' {
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
