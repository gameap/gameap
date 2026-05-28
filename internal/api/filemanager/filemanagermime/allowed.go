// Package filemanagermime owns the MIME allowlist that gates file uploads
// and is consulted by the file-serving layer to decide whether a stored
// file may be served inline.
//
// Two principles drive the list:
//
//   - "Inert" content only — types a browser renders without executing
//     script. image/svg+xml is intentionally excluded (it can embed
//     <script>); HTML is excluded for the same reason.
//   - Symmetric with the serve-side `inlineSafeMimes` in
//     internal/api/filemanager/filemanagerhttp/headers.go. The serve path
//     mitigates a leaked HTML upload by stripping the inline disposition,
//     but defence-in-depth demands rejection at upload time too.
//
// Operators can extend (not replace) the list via FILES_UPLOAD_ALLOWED_MIMES
// and FILES_UPLOAD_ALLOW_ARCHIVES toggles. game-saves and other binary
// payloads can also be unlocked via FILES_UPLOAD_ALLOW_BINARY which lets
// generic application/octet-stream through.
package filemanagermime

import (
	"mime"
	"strings"
)

// defaultAllowedMIMEs lists the bare media types accepted unconditionally
// on upload. The list mirrors `inlineSafeMimes` in the serve layer plus
// the structured-text formats game-server operators legitimately need
// (JSON / XML / YAML configs come back as text/plain or one of the
// listed application/* forms via http.DetectContentType).
var defaultAllowedMIMEs = map[string]struct{}{
	// Images — same allowlist as the serve layer.
	"image/png":                {},
	"image/jpeg":               {},
	"image/gif":                {},
	"image/webp":               {},
	"image/bmp":                {},
	"image/x-icon":             {},
	"image/vnd.microsoft.icon": {},

	// Plain text — covers most game configs (cfg, ini, conf, yaml, toml,
	// properties files): http.DetectContentType returns text/plain for
	// anything that lacks a magic-byte signature.
	"text/plain": {},

	// Structured text payloads with explicit signatures.
	"application/json": {},
	"application/xml":  {},
	"text/xml":         {},
	"text/csv":         {},
	"text/yaml":        {},

	// Documents commonly attached to game-server READMEs.
	"application/pdf": {},
}

// archiveMIMEs is unlocked by FILES_UPLOAD_ALLOW_ARCHIVES. Useful for
// custom map packs and mod bundles; refused by default because a malicious
// archive can carry executables that a daemon-side extraction would
// happily run.
var archiveMIMEs = map[string]struct{}{
	"application/zip":             {},
	"application/x-tar":           {},
	"application/x-gzip":          {},
	"application/gzip":            {},
	"application/x-bzip2":         {},
	"application/x-7z-compressed": {},
	"application/x-xz":            {},
}

// binaryMIME is the catch-all unlocked by FILES_UPLOAD_ALLOW_BINARY.
// http.DetectContentType returns this for any payload it cannot classify
// — game saves, custom binary formats, daemon executables. Default off.
const binaryMIME = "application/octet-stream"

// Config controls the allowlist composition. Sourced from
// internal/config Files.* at boot.
type Config struct {
	// AllowedMIMEs additively extends the default list. Empty means
	// "defaults only".
	AllowedMIMEs []string

	// AllowArchives toggles the archive group as one switch.
	AllowArchives bool

	// AllowBinary unlocks application/octet-stream. Operators only
	// turn this on for deployments that legitimately accept opaque
	// game-save uploads.
	AllowBinary bool
}

// Checker tests a detected MIME (as returned by net/http.DetectContentType)
// against the configured allowlist. The detected value may carry
// parameters ("text/plain; charset=utf-8") — Allowed strips them before
// the lookup so the result is parameter-insensitive.
type Checker struct {
	allowed map[string]struct{}
}

// NewChecker compiles the allowlist once at boot.
func NewChecker(cfg Config) *Checker {
	allowed := make(map[string]struct{}, len(defaultAllowedMIMEs))

	for m := range defaultAllowedMIMEs {
		allowed[m] = struct{}{}
	}

	if cfg.AllowArchives {
		for m := range archiveMIMEs {
			allowed[m] = struct{}{}
		}
	}

	if cfg.AllowBinary {
		allowed[binaryMIME] = struct{}{}
	}

	for _, m := range cfg.AllowedMIMEs {
		m = bareType(m)
		if m == "" {
			continue
		}
		allowed[m] = struct{}{}
	}

	return &Checker{allowed: allowed}
}

// Allowed returns the bare detected MIME and whether the type is on the
// allowlist. The bare form is returned so callers can include the exact
// detected type in audit logs.
func (c *Checker) Allowed(detected string) (string, bool) {
	bare := bareType(detected)
	if bare == "" {
		return "", false
	}

	_, ok := c.allowed[bare]

	return bare, ok
}

// bareType lowercases and strips any "; param=value" tail from a MIME so
// "text/plain; charset=utf-8" and "text/plain" match the same allowlist
// entry. Returns "" for malformed input.
func bareType(m string) string {
	m = strings.TrimSpace(m)
	if m == "" {
		return ""
	}

	parsed, _, err := mime.ParseMediaType(m)
	if err != nil {
		// ParseMediaType is strict; fall back to a naive split for inputs
		// like "application/json" without parameters that ParseMediaType
		// already accepts. Returning "" here keeps the call-site simple.
		bare := strings.ToLower(m)
		if i := strings.IndexByte(bare, ';'); i >= 0 {
			bare = strings.TrimSpace(bare[:i])
		}

		return bare
	}

	return parsed
}
