package telemetry

import (
	"strings"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/domain"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/prometheus/client_golang/prometheus"
)

const pluginSubsystem = "plugin"

// hostModulePrefix is stripped from the module label: "gameap-nodefs" is
// reported as "nodefs".
const hostModulePrefix = "gameap-"

// PluginLister is the manager view the collector needs.
type PluginLister interface {
	GetPlugins() []*pkgplugin.LoadedPlugin
}

// BacklogReporter is the dispatcher view the collector needs.
type BacklogReporter interface {
	AsyncBacklog() int
}

// SyncReporter is the multi-instance reconciler view the collector needs.
type SyncReporter interface {
	Pending() int
}

// PluginMetrics implements pkgplugin.Observer and the host libraries'
// HostCallObserver, and collects per-plugin gauges on scrape. The plugin
// label is the compact form of the database ID, the same id the admin API
// uses.
type PluginMetrics struct {
	hostCalls        *prometheus.CounterVec
	hostCallDuration *prometheus.HistogramVec
	hostCallsDenied  *prometheus.CounterVec

	guestCalls        *prometheus.CounterVec
	guestCallDuration *prometheus.HistogramVec

	events   *prometheus.CounterVec
	disabled *prometheus.CounterVec

	backlog     prometheus.GaugeFunc
	syncPasses  *prometheus.CounterVec
	syncPending prometheus.GaugeFunc
	memoryDesc  *prometheus.Desc
	enabledDesc *prometheus.Desc
	healthDesc  *prometheus.Desc

	mu         sync.Mutex
	plugins    PluginLister
	backlogSrc BacklogReporter
	syncSrc    SyncReporter
	// lastMemory keeps the last observed memory size per plugin: MemorySize
	// is unavailable while a guest call is in flight, and a gauge that
	// blinks on busy plugins is worse than one a scrape behind.
	lastMemory map[string]uint64
}

// NewPluginMetrics builds and registers the plugin collectors. plugins and
// backlog may be nil (tests); the gauges they feed are then not collected.
func NewPluginMetrics(
	registry *Registry,
	plugins PluginLister,
	backlog BacklogReporter,
	sync SyncReporter,
) *PluginMetrics {
	m := &PluginMetrics{
		hostCalls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace, Subsystem: pluginSubsystem, Name: "host_calls_total",
			Help: "Host library functions invoked by plugins.",
		}, []string{"plugin", "module", "rpc", "result"}),
		hostCallDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace, Subsystem: pluginSubsystem, Name: "host_call_duration_seconds",
			Help:    "Time spent in host library functions invoked by plugins.",
			Buckets: prometheus.ExponentialBuckets(0.0005, 2, 14),
		}, []string{"plugin", "module"}),
		hostCallsDenied: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace, Subsystem: pluginSubsystem, Name: "host_calls_denied_total",
			Help: "Host library calls refused by the guard (missing grant or rate limit).",
		}, []string{"plugin", "module", "rpc", "reason"}),
		guestCalls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace, Subsystem: pluginSubsystem, Name: "guest_calls_total",
			Help: "Calls from the panel into plugin exports.",
		}, []string{"plugin", "export", "result"}),
		guestCallDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace, Subsystem: pluginSubsystem, Name: "guest_call_duration_seconds",
			Help:    "Time spent inside plugin exports, including the wait for the per-plugin call gate.",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 16),
		}, []string{"plugin", "export"}),
		events: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace, Subsystem: pluginSubsystem, Name: "events_dispatched_total",
			Help: "Event deliveries to plugin subscribers by outcome; dropped counts events lost to a full async backlog.",
		}, []string{"type", "result"}),
		disabled: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace, Subsystem: pluginSubsystem, Name: "disabled_total",
			Help: "Plugins disabled at runtime by reason.",
		}, []string{"plugin", "reason"}),
		memoryDesc: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, pluginSubsystem, "memory_bytes"),
			"Linear memory of the plugin module on this instance (last observed value while a call is in flight).",
			[]string{"plugin"}, nil),
		enabledDesc: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, pluginSubsystem, "enabled"),
			"1 when the plugin is loaded and receiving events on this instance, 0 when disabled.",
			[]string{"plugin"}, nil),
		healthDesc: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, pluginSubsystem, "health"),
			"Self-reported plugin health on this instance (gameap-host ReportStatus): "+
				"1 for the reported status, 0 for the others.",
			[]string{"plugin", "status"}, nil),
		syncPasses: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace, Subsystem: pluginSubsystem, Name: "sync_passes_total",
			Help: "Multi-instance reconcile passes on this instance by result.",
		}, []string{"result"}),
		plugins:    plugins,
		backlogSrc: backlog,
		syncSrc:    sync,
		lastMemory: make(map[string]uint64),
	}

	m.backlog = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: Namespace, Subsystem: pluginSubsystem, Name: "async_backlog",
		Help: "Fire-and-forget event deliveries in flight or queued on this instance.",
	}, func() float64 {
		if m.backlogSrc == nil {
			return 0
		}

		return float64(m.backlogSrc.AsyncBacklog())
	})

	m.syncPending = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: Namespace, Subsystem: pluginSubsystem, Name: "sync_pending",
		Help: "Plugins the multi-instance reconciler could not bring in line with the database on this instance.",
	}, func() float64 {
		if m.syncSrc == nil {
			return 0
		}

		return float64(m.syncSrc.Pending())
	})

	registry.MustRegister(
		m.hostCalls, m.hostCallDuration, m.hostCallsDenied,
		m.guestCalls, m.guestCallDuration,
		m.events, m.disabled, m.backlog, m.syncPasses, m.syncPending, m,
	)

	return m
}

// SyncPass implements pluginsync.PassObserver.
func (m *PluginMetrics) SyncPass(result string) {
	m.syncPasses.WithLabelValues(result).Inc()
}

// PluginLabel is the metric label of a plugin: the compact database ID.
func PluginLabel(dbID uint64) string {
	return pkgplugin.CompactPluginID(domain.Uint64ID(dbID))
}

func moduleLabel(module string) string {
	return strings.TrimPrefix(module, hostModulePrefix)
}

// GuestCall implements pkgplugin.Observer.
func (m *PluginMetrics) GuestCall(pluginID uint64, export string, duration time.Duration, result string) {
	if pluginID == 0 {
		return
	}

	label := PluginLabel(pluginID)
	m.guestCalls.WithLabelValues(label, export, result).Inc()
	m.guestCallDuration.WithLabelValues(label, export).Observe(duration.Seconds())
}

// HostCall implements pkgplugin.Observer.
func (m *PluginMetrics) HostCall(pluginID uint64, module, function string, duration time.Duration, panicked bool) {
	if pluginID == 0 {
		return
	}

	result := "ok"
	if panicked {
		result = "panic"
	}

	label := PluginLabel(pluginID)
	m.hostCalls.WithLabelValues(label, moduleLabel(module), function, result).Inc()
	m.hostCallDuration.WithLabelValues(label, moduleLabel(module)).Observe(duration.Seconds())
}

// EventDispatched implements pkgplugin.Observer.
func (m *PluginMetrics) EventDispatched(eventType proto.EventType, result string) {
	name, ok := proto.EventType_name[int32(eventType)]
	if !ok {
		name = "unknown"
	}

	m.events.WithLabelValues(strings.TrimPrefix(name, "EVENT_TYPE_"), result).Inc()
}

// HostCallDenied implements the host libraries' HostCallObserver.
func (m *PluginMetrics) HostCallDenied(pluginID uint64, module, function, reason string) {
	if pluginID == 0 {
		return
	}

	m.hostCallsDenied.WithLabelValues(PluginLabel(pluginID), moduleLabel(module), function, reason).Inc()
}

// OnPluginDisabled is a pkgplugin.DisableHook; the reason label keeps only
// the stable prefix of the disable reason (the detail in parentheses names
// the event or route and would explode the label set).
func (m *PluginMetrics) OnPluginDisabled(_ string, dbID uint64, reason string) {
	if dbID == 0 {
		return
	}

	if i := strings.Index(reason, " ("); i >= 0 {
		reason = reason[:i]
	}

	m.disabled.WithLabelValues(PluginLabel(dbID), reason).Inc()
}

// Describe implements prometheus.Collector.
func (m *PluginMetrics) Describe(ch chan<- *prometheus.Desc) {
	ch <- m.memoryDesc
	ch <- m.enabledDesc
	ch <- m.healthDesc
}

// healthStatuses are the label values of the health gauge, emitted as a
// state set so a dashboard can select on the status label.
var healthStatuses = []pkgplugin.HealthStatus{
	pkgplugin.HealthHealthy, pkgplugin.HealthDegraded, pkgplugin.HealthUnhealthy,
}

// Collect implements prometheus.Collector.
func (m *PluginMetrics) Collect(ch chan<- prometheus.Metric) {
	if m.plugins == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, plugin := range m.plugins.GetPlugins() {
		if plugin.DBID == 0 {
			continue
		}

		label := PluginLabel(plugin.DBID)

		enabled := 0.0
		if plugin.IsEnabled() {
			enabled = 1
		}

		ch <- prometheus.MustNewConstMetric(m.enabledDesc, prometheus.GaugeValue, enabled, label)

		if size, ok := plugin.MemorySize(); ok {
			m.lastMemory[label] = size
		}

		if size, ok := m.lastMemory[label]; ok {
			ch <- prometheus.MustNewConstMetric(m.memoryDesc, prometheus.GaugeValue, float64(size), label)
		}

		if report, ok := plugin.Health(); ok {
			for _, status := range healthStatuses {
				value := 0.0
				if report.Status == status {
					value = 1
				}

				ch <- prometheus.MustNewConstMetric(m.healthDesc, prometheus.GaugeValue, value, label, status.String())
			}
		}
	}
}
