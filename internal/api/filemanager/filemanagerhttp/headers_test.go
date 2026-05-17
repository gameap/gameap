// Security tests for the centralized file-manager response headers.
//
// OWASP API Security Top 10:2023 — API1:2023 Broken Object Level
// Authorization is the entry point (a user reads another server's stored
// file), but the concrete weakness exercised here is Stored XSS: a
// user-uploaded HTML/SVG/JS file served inline in the credentialed panel
// origin could run script and steal another user's session. SafeContentHeaders
// forces such content to download with sniffing disabled and a sandbox CSP.
// See security review finding #5.
package filemanagerhttp_test

import (
	"net/http"
	"testing"

	"github.com/gameap/gameap/internal/api/filemanager/filemanagerhttp"
	"github.com/stretchr/testify/assert"
)

// TestSafeContentHeaders_Disposition — OWASP API1:2023 Broken Object Level
// Authorization / Stored XSS: only an inert allowlist may render inline;
// everything else (notably text/html and image/svg+xml) is forced to an
// opaque attachment so it cannot execute in the panel origin.
func TestSafeContentHeaders_Disposition(t *testing.T) {
	const fn = "report"

	tests := []struct {
		name            string
		daemonMime      string
		wantContentType string
		wantDisposition string
	}{
		{
			name:            "empty_mime_is_opaque_attachment",
			daemonMime:      "",
			wantContentType: "application/octet-stream",
			wantDisposition: "attachment",
		},
		{
			name:            "html_is_forced_to_attachment_not_rendered",
			daemonMime:      "text/html",
			wantContentType: "application/octet-stream",
			wantDisposition: "attachment",
		},
		{
			name:            "html_with_charset_param_is_forced_to_attachment",
			daemonMime:      "text/html; charset=utf-8",
			wantContentType: "application/octet-stream",
			wantDisposition: "attachment",
		},
		{
			name:            "svg_is_forced_to_attachment_can_embed_script",
			daemonMime:      "image/svg+xml",
			wantContentType: "application/octet-stream",
			wantDisposition: "attachment",
		},
		{
			name:            "javascript_is_forced_to_attachment",
			daemonMime:      "application/javascript",
			wantContentType: "application/octet-stream",
			wantDisposition: "attachment",
		},
		{
			name:            "xhtml_is_forced_to_attachment",
			daemonMime:      "application/xhtml+xml",
			wantContentType: "application/octet-stream",
			wantDisposition: "attachment",
		},
		{
			name:            "video_is_forced_to_attachment_not_inline",
			daemonMime:      "video/mp4",
			wantContentType: "application/octet-stream",
			wantDisposition: "attachment",
		},
		{
			name:            "unknown_image_subtype_is_forced_to_attachment_not_inline",
			daemonMime:      "image/x-unknown-scriptable",
			wantContentType: "application/octet-stream",
			wantDisposition: "attachment",
		},
		{
			name:            "png_renders_inline",
			daemonMime:      "image/png",
			wantContentType: "image/png",
			wantDisposition: "inline",
		},
		{
			name:            "png_with_charset_param_renders_inline_as_bare_mime",
			daemonMime:      "image/png; charset=binary",
			wantContentType: "image/png",
			wantDisposition: "inline",
		},
		{
			name:            "plain_text_renders_inline",
			daemonMime:      "text/plain",
			wantContentType: "text/plain",
			wantDisposition: "inline",
		},
		{
			name:            "pdf_renders_inline",
			daemonMime:      "application/pdf",
			wantContentType: "application/pdf",
			wantDisposition: "inline",
		},
		{
			name:            "uppercase_mime_is_normalized_and_renders_inline",
			daemonMime:      "IMAGE/PNG",
			wantContentType: "image/png",
			wantDisposition: "inline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			h := http.Header{}

			// ACT
			filemanagerhttp.SafeContentHeaders(h, fn, tt.daemonMime)

			// ASSERT
			assert.Equal(t, tt.wantContentType, h.Get("Content-Type"),
				"Content-Type must be the inert daemon mime only for the inline allowlist")
			assert.True(t,
				startsWith(h.Get("Content-Disposition"), tt.wantDisposition),
				"Content-Disposition must start with %q, got %q",
				tt.wantDisposition, h.Get("Content-Disposition"))

			// Hardening headers are unconditional regardless of disposition.
			assert.Equal(t, "nosniff", h.Get("X-Content-Type-Options"),
				"nosniff must always be set so the browser cannot MIME-sniff to HTML")
			assert.Equal(t, "sandbox", h.Get("Content-Security-Policy"),
				"sandbox CSP must always be set so inline content cannot script")
		})
	}
}

// TestSafeContentHeaders_FilenameEncoding — OWASP API1:2023 / header
// injection: a filename containing quotes, backslashes, control bytes or
// non-ASCII must not break the Content-Disposition header or inject extra
// parameters. The ASCII fallback is sanitized and the exact bytes are carried
// in the RFC 5987 filename* parameter.
func TestSafeContentHeaders_FilenameEncoding(t *testing.T) {
	tests := []struct {
		name               string
		filename           string
		wantContains       []string
		wantNotContainsRaw string
	}{
		{
			name:     "ascii_plain_filename_round_trips",
			filename: "server.properties",
			wantContains: []string{
				`filename=server.properties`,
				`filename*=UTF-8''server.properties`,
			},
		},
		{
			name:     "double_quote_is_stripped_from_ascii_fallback",
			filename: `a"b.txt`,
			wantContains: []string{
				`filename=a_b.txt`,
				`filename*=UTF-8''`,
			},
			wantNotContainsRaw: `a"b.txt`,
		},
		{
			name:     "backslash_is_stripped_from_ascii_fallback",
			filename: `a\b.txt`,
			wantContains: []string{
				`filename=a_b.txt`,
				`filename*=UTF-8''`,
			},
			wantNotContainsRaw: `a\b.txt`,
		},
		{
			// The CRLF and the injected "Content-Type:" must be percent-encoded
			// in filename* and the control bytes stripped from the ASCII
			// fallback, so no raw header break or extra real header can appear.
			name:     "header_break_attempt_cannot_inject_a_second_parameter",
			filename: "x.txt\r\nContent-Type: text/html",
			wantContains: []string{
				`x.txt%0D%0AContent-Type`,
			},
			wantNotContainsRaw: "\r\n",
		},
		{
			name:     "unicode_filename_is_percent_encoded_in_filename_star",
			filename: "résumé.pdf",
			wantContains: []string{
				`filename*=UTF-8''r%C3%A9sum%C3%A9.pdf`,
			},
		},
		{
			// Every non-ASCII rune is replaced by '_' in the ASCII fallback
			// (not dropped), so the fallback is non-empty and the exact bytes
			// are still recoverable from the percent-encoded filename*.
			name:     "all_non_ascii_filename_is_underscored_in_ascii_and_encoded_in_star",
			filename: "файл",
			wantContains: []string{
				`filename=____`,
				`filename*=UTF-8''%D1%84%D0%B0%D0%B9%D0%BB`,
			},
		},
		{
			// An empty filename (the only input whose ASCII fallback collapses
			// to the empty string) uses the literal "download" token so the
			// header parameter is never empty.
			name:     "empty_filename_falls_back_to_download_token",
			filename: "",
			wantContains: []string{
				`filename=download`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			h := http.Header{}

			// ACT — empty mime so disposition is the (safe) attachment default.
			filemanagerhttp.SafeContentHeaders(h, tt.filename, "")

			// ASSERT
			cd := h.Get("Content-Disposition")
			for _, want := range tt.wantContains {
				assert.Contains(t, cd, want,
					"Content-Disposition must contain %q, got %q", want, cd)
			}
			if tt.wantNotContainsRaw != "" {
				assert.NotContains(t, cd, tt.wantNotContainsRaw,
					"raw unsafe sequence %q must not appear unescaped in the header",
					tt.wantNotContainsRaw)
			}
			assert.NotContains(t, cd, "\n",
				"a newline must never reach the Content-Disposition header value")
		})
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
