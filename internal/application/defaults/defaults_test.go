package defaults

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These are intentionally light change-detector tests over compile-time
// constants. Version and BuildDate are asserted non-empty rather than equal to
// their source defaults because CI overrides them via -ldflags; non-emptiness is
// the stable invariant. StoragePath is platform-selected at build time, so the
// expected value is chosen via runtime.GOOS to keep the test green on both the
// local darwin machine and the linux CI runner.

func TestVersionDefaultsAreNonEmpty(t *testing.T) {
	assert.NotEmpty(t, Version, "Version must always carry a value (default or -ldflags override)")
	assert.NotEmpty(t, BuildDate, "BuildDate must always carry a value (default or -ldflags override)")
}

func TestStoragePathMatchesPlatform(t *testing.T) {
	want := map[string]string{
		"darwin":  "",
		"linux":   "/var/www/gameap/storage/app",
		"windows": "C:\\gameap\\web\\storage\\app",
	}

	expected, ok := want[runtime.GOOS]
	require.True(t, ok, "no expected StoragePath defined for GOOS %q", runtime.GOOS)

	assert.Equal(t, expected, StoragePath, "StoragePath must match the platform-specific default")
}
