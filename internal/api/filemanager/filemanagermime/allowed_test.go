// API Security Tests for OWASP API Security Top 10:2023.
// Category: API6:2023 — Unrestricted Access to Sensitive Business Flows
// (file upload is the canonical example) plus API8:2023 — Security
// Misconfiguration (the operator toggles).
//
// Pins ASVS_L2 C-8 (file-upload MIME allowlist): the default Checker
// accepts inert image / text / pdf / structured payloads, rejects HTML
// and SVG even when the upload claims to be a PNG, and additively
// honours operator-supplied AllowedMIMEs / AllowArchives / AllowBinary
// toggles.
package filemanagermime_test

import (
	"testing"

	"github.com/gameap/gameap/internal/api/filemanager/filemanagermime"
	"github.com/stretchr/testify/assert"
)

func TestChecker_Defaults_AcceptsInertTypes(t *testing.T) {
	t.Parallel()

	c := filemanagermime.NewChecker(filemanagermime.Config{})

	for _, m := range []string{
		"image/png",
		"image/jpeg",
		"image/gif",
		"image/webp",
		"text/plain",
		"text/plain; charset=utf-8",
		"application/json",
		"application/xml",
		"text/csv",
		"application/pdf",
	} {
		bare, ok := c.Allowed(m)
		assert.True(t, ok, "default Checker must accept inert type %q", m)
		assert.NotEmpty(t, bare)
	}
}

func TestChecker_Defaults_RejectsExecutableContent(t *testing.T) {
	t.Parallel()

	c := filemanagermime.NewChecker(filemanagermime.Config{})

	for _, m := range []string{
		"text/html", // <script> capable
		"text/html; charset=utf-8",
		"image/svg+xml", // scriptable
		"application/javascript",
		"application/x-msdownload", // .exe
		"application/octet-stream", // unknown binary, default OFF
		"application/x-sh",
		"application/x-php",
		"application/zip", // archives off by default
	} {
		_, ok := c.Allowed(m)
		assert.False(t, ok, "default Checker must reject %q", m)
	}
}

func TestChecker_AllowArchives_UnlocksArchiveGroup(t *testing.T) {
	t.Parallel()

	c := filemanagermime.NewChecker(filemanagermime.Config{AllowArchives: true})

	for _, m := range []string{
		"application/zip",
		"application/x-tar",
		"application/gzip",
		"application/x-gzip",
		"application/x-bzip2",
		"application/x-7z-compressed",
	} {
		_, ok := c.Allowed(m)
		assert.True(t, ok, "AllowArchives=true must accept %q", m)
	}
}

func TestChecker_AllowBinary_UnlocksOctetStreamOnly(t *testing.T) {
	t.Parallel()

	c := filemanagermime.NewChecker(filemanagermime.Config{AllowBinary: true})

	bare, ok := c.Allowed("application/octet-stream")
	assert.True(t, ok)
	assert.Equal(t, "application/octet-stream", bare)

	// AllowBinary must NOT additionally unlock executable types or
	// archives — only the catch-all octet-stream.
	_, ok = c.Allowed("application/x-msdownload")
	assert.False(t, ok)
}

func TestChecker_OperatorExtrasAreAdditive(t *testing.T) {
	t.Parallel()

	c := filemanagermime.NewChecker(filemanagermime.Config{
		AllowedMIMEs: []string{"video/mp4", "audio/mpeg"},
	})

	// Defaults still accepted.
	_, ok := c.Allowed("image/png")
	assert.True(t, ok)

	// Operator extras accepted.
	_, ok = c.Allowed("video/mp4")
	assert.True(t, ok)
	_, ok = c.Allowed("audio/mpeg")
	assert.True(t, ok)
}

func TestChecker_BareTypeIsParameterInsensitive(t *testing.T) {
	t.Parallel()

	c := filemanagermime.NewChecker(filemanagermime.Config{})

	bare, ok := c.Allowed("text/plain; charset=utf-8; boundary=---")
	assert.True(t, ok)
	assert.Equal(t, "text/plain", bare, "Allowed must return the bare type, not the parameter list")
}

func TestChecker_MalformedInputIsRejected(t *testing.T) {
	t.Parallel()

	c := filemanagermime.NewChecker(filemanagermime.Config{})

	for _, m := range []string{
		"",
		"   ",
		"not-a-mime",
	} {
		_, ok := c.Allowed(m)
		assert.False(t, ok, "malformed MIME %q must be rejected", m)
	}
}
