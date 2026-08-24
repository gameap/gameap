package plugin

import (
	"strconv"
	"strings"

	"github.com/gameap/gameap/pkg/plugin/proto"
)

// shortErrorMaxLen bounds the error detail embedded in a disable reason;
// wazero errors can carry a multi-line stack trace.
const shortErrorMaxLen = 200

// Stable prefixes of the reasons a plugin is disabled at runtime. The detail
// in parentheses (event type, route, task name) is appended by the call site.
const (
	DisableReasonEventTimeout     = "event handler timed out"
	DisableReasonHTTPTimeout      = "http handler timed out"
	DisableReasonScheduledTimeout = "scheduled task timed out"
	DisableReasonArchiveTimeout   = "archive callback timed out"
	// DisableReasonGuestExited: the guest terminated its own module
	// (a Go panic ends in proc_exit), so every further call would fail.
	DisableReasonGuestExited = "guest module exited"
)

// DisableHook is notified when a registered plugin is disabled with a
// reason. It runs on its own goroutine with no manager or dispatcher lock
// held, so it may reload the plugin; dbID is 0 for transient loads.
type DisableHook func(pluginID string, dbID uint64, reason string)

// EventTimeoutReason names the event whose handler overran its budget.
func EventTimeoutReason(eventType proto.EventType) string {
	name, ok := proto.EventType_name[int32(eventType)]
	if !ok {
		name = strconv.Itoa(int(eventType))
	}

	return DisableReasonEventTimeout + " (" + strings.TrimPrefix(name, "EVENT_TYPE_") + ")"
}

// DisableWithReason stops event and HTTP delivery like Disable and records
// why. The first reason wins and the disable hook fires once; a plugin
// already disabled silently (unload, shutdown) keeps no reason.
func (p *LoadedPlugin) DisableWithReason(reason string) {
	if !p.disabled.CompareAndSwap(false, true) {
		return
	}

	p.disableReason.Store(&reason)

	if p.onDisabled == nil {
		return
	}

	pluginID := ""
	if p.Info != nil {
		pluginID = p.Info.Id
	}

	go p.onDisabled(pluginID, p.DBID, reason)
}

// DisabledReason reports why the plugin was disabled at runtime; false when
// it is enabled or was disabled without a reason.
func (p *LoadedPlugin) DisabledReason() (string, bool) {
	reason := p.disableReason.Load()
	if reason == nil {
		return "", false
	}

	return *reason, true
}

// MemorySize reports the guest's current linear memory in bytes. It is a
// snapshot taken between guest calls; false when the plugin is disabled, a
// call is in flight, or the instance does not expose its module.
func (p *LoadedPlugin) MemorySize() (uint64, bool) {
	if p.disabled.Load() {
		return 0, false
	}

	sizer, ok := p.Instance.(interface{ MemorySize() (uint64, bool) })
	if !ok {
		return 0, false
	}

	return sizer.MemorySize()
}

// shortErrorText keeps the first line of an error, capped in length, for
// reasons that end up in the database and the admin UI.
func shortErrorText(err error) string {
	if err == nil {
		return ""
	}

	text, _, _ := strings.Cut(err.Error(), "\n")
	text = strings.TrimSpace(text)

	if len(text) > shortErrorMaxLen {
		text = text[:shortErrorMaxLen]
	}

	return text
}
