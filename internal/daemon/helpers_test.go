package daemon

import (
	"context"
	"testing"
	"time"
)

// testContext returns a 30-second context bound to the test's Cleanup, so
// goroutines exit when the test finishes regardless of pass/fail.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	return ctx
}
