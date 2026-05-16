package audit

import "context"

// Logger records security-relevant audit events. Implementations must be
// safe for concurrent use and must not block the request path.
type Logger interface {
	Record(ctx context.Context, event Event)
}
