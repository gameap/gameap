package middlewares

import (
	"context"
	"sync"

	"github.com/gameap/gameap/internal/audit"
)

// auditCapture is a concurrency-safe audit.Logger that records every event a
// middleware emits during a request. It is the package-local test double used
// by the auth / daemon / login-rate-limit security tests to assert that the
// wired call sites actually emit the expected audit.Event (mirrors the one in
// internal/api/router_security_auditlog_test.go).
type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *auditCapture) snapshot() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]audit.Event(nil), a.events...)
}

// findEvent returns the first recorded event of the given type.
func findEvent(events []audit.Event, t audit.EventType) (audit.Event, bool) {
	for _, e := range events {
		if e.Type == t {
			return e, true
		}
	}

	return audit.Event{}, false
}

// countEvents returns how many recorded events have the given type.
func countEvents(events []audit.Event, t audit.EventType) int {
	n := 0
	for _, e := range events {
		if e.Type == t {
			n++
		}
	}

	return n
}

// extraString returns the string value of the first Extra attr with key.
func extraString(e audit.Event, key string) (string, bool) {
	for _, a := range e.Extra {
		if a.Key == key {
			return a.Value.String(), true
		}
	}

	return "", false
}
