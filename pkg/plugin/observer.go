package plugin

import (
	"time"

	"github.com/gameap/gameap/pkg/plugin/proto"
)

// Guest call results reported to the Observer.
const (
	GuestCallResultOK      = "ok"
	GuestCallResultError   = "error"
	GuestCallResultTimeout = "timeout"
	GuestCallResultBusy    = "busy"
)

// Event dispatch results reported to the Observer.
const (
	EventResultHandled   = "handled"
	EventResultIgnored   = "ignored"
	EventResultCancelled = "cancelled"
	EventResultError     = "error"
	EventResultDropped   = "dropped"
)

// Observer receives runtime signals from the plugin system so the panel can
// expose them as metrics. Implementations must be cheap and non-blocking:
// every hook runs on the calling goroutine, inside guest and host calls.
// pluginID is the database ID the plugin was loaded for (0 = transient).
type Observer interface {
	// GuestCall reports one call into a guest export (handle_event,
	// handle_http_request, ...). result is one of the GuestCallResult* values.
	GuestCall(pluginID uint64, export string, duration time.Duration, result string)
	// HostCall reports one host-library function invoked by a guest; panicked
	// is true when the host function panicked (the guest call then traps).
	HostCall(pluginID uint64, module, function string, duration time.Duration, panicked bool)
	// EventDispatched reports the outcome of delivering one event to one
	// subscriber, or a drop when the async backlog is full.
	EventDispatched(eventType proto.EventType, result string)
}

// NopObserver ignores every signal.
type NopObserver struct{}

func (NopObserver) GuestCall(uint64, string, time.Duration, string)      {}
func (NopObserver) HostCall(uint64, string, string, time.Duration, bool) {}
func (NopObserver) EventDispatched(proto.EventType, string)              {}

// observerOrNop keeps call sites free of nil checks.
func observerOrNop(o Observer) Observer {
	if o == nil {
		return NopObserver{}
	}

	return o
}
