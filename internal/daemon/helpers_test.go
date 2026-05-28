package daemon

import (
	"context"
	"testing"
	"time"
)

// testInstanceID is the shared dispatcher instance identifier used across the
// in-memory pub/sub round-trip tests.
const testInstanceID = "test-instance"

// testContext returns a 30-second context bound to the test's Cleanup, so
// goroutines exit when the test finishes regardless of pass/fail.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	return ctx
}
