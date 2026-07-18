package ws

import (
	"log/slog"
	"os"
	"testing"
)

// TestMain silences the package-wide fallback logger: NewHub(nil) and
// NewClient(..., nil) fall back to slog.Default(), and the hub stress tests
// intentionally overflow client buffers — the ~100k dropped-message warnings
// written to stderr under -race slow CI runs into the go test timeout.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))

	os.Exit(m.Run())
}
