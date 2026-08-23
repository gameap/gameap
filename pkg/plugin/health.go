package plugin

import (
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/plugin/proto"
)

// HealthStatus is a plugin's self-reported condition (gameap-host
// ReportStatus).
type HealthStatus uint8

const (
	HealthUnknown HealthStatus = iota
	HealthHealthy
	HealthDegraded
	HealthUnhealthy
)

// String renders the status the way the admin API and the metrics label it.
func (s HealthStatus) String() string {
	switch s {
	case HealthHealthy:
		return "healthy"
	case HealthDegraded:
		return "degraded"
	case HealthUnhealthy:
		return "unhealthy"
	case HealthUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// Bounds of a health report; what a guest sends beyond them is cut, never
// refused, so a verbose plugin still gets its status across.
const (
	MaxHealthMessageLen     = 512
	MaxHealthDetails        = 16
	MaxHealthDetailKeyLen   = 64
	MaxHealthDetailValueLen = 256
)

// HealthReport is one self-diagnosis a plugin published on this panel
// instance.
type HealthReport struct {
	Status     HealthStatus
	Message    string
	Details    map[string]string
	ReportedAt time.Time
}

// bounded returns a copy of the report cut to the documented limits, with
// the details sorted by key so the cut is deterministic.
func (r HealthReport) bounded() HealthReport {
	if len(r.Message) > MaxHealthMessageLen {
		r.Message = r.Message[:MaxHealthMessageLen]
	}

	if len(r.Details) == 0 {
		r.Details = nil

		return r
	}

	keys := make([]string, 0, len(r.Details))
	for key := range r.Details {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	details := make(map[string]string, min(len(keys), MaxHealthDetails))

	for _, key := range keys {
		if len(details) >= MaxHealthDetails {
			break
		}

		value := r.Details[key]
		if len(value) > MaxHealthDetailValueLen {
			value = value[:MaxHealthDetailValueLen]
		}

		// An over-long key is dropped rather than cut: cutting could merge
		// two distinct keys into one.
		if len(key) > MaxHealthDetailKeyLen {
			continue
		}

		details[key] = value
	}

	r.Details = details

	return r
}

// SetHealth stores the plugin's latest self-reported status; the status
// change is logged once so operators see transitions in the panel log.
func (p *LoadedPlugin) SetHealth(report HealthReport) {
	report = report.bounded()
	if report.ReportedAt.IsZero() {
		report.ReportedAt = time.Now()
	}

	previous := p.health.Swap(&report)

	if previous == nil || previous.Status != report.Status {
		// A report sent from Initialize arrives before Info is set; the
		// compact database id names the plugin the way the admin API does.
		pluginID := ""
		if p.Info != nil {
			pluginID = p.Info.Id
		} else if p.DBID != 0 {
			pluginID = CompactPluginID(domain.Uint64ID(p.DBID))
		}

		slog.Info("plugin reported health status",
			slog.String("plugin_id", pluginID),
			slog.Uint64("db_id", p.DBID),
			slog.String("status", report.Status.String()),
			slog.String("message", report.Message))
	}
}

// Health reports the plugin's latest self-reported status on this instance;
// false when the plugin never reported one.
func (p *LoadedPlugin) Health() (HealthReport, bool) {
	report := p.health.Load()
	if report == nil {
		return HealthReport{}, false
	}

	return *report, true
}

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
