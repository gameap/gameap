package telemetry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubBacklog struct{ depth int }

func (s stubBacklog) AsyncBacklog() int { return s.depth }

func TestPluginMetrics_counts_signals(t *testing.T) {
	t.Parallel()

	registry := New()
	metrics := NewPluginMetrics(registry, nil, stubBacklog{depth: 3}, nil)

	metrics.HostCall(42, "gameap-nodecmd", "execute_command", 20*time.Millisecond, false)
	metrics.HostCall(42, "gameap-nodecmd", "execute_command", 5*time.Millisecond, true)
	metrics.HostCallDenied(42, "gameap-nodecmd", "execute_command", "permission")
	metrics.GuestCall(42, "plugin_service_handle_event", time.Millisecond, "ok")
	metrics.GuestCall(42, "plugin_service_handle_event", time.Second, "timeout")
	metrics.EventDispatched(proto.EventType_EVENT_TYPE_SERVER_POST_START, "handled")
	metrics.OnPluginDisabled("plugin", 42, "event handler timed out (SERVER_POST_START)")

	// Transient loads are not labelled.
	metrics.HostCall(0, "gameap-log", "log", time.Millisecond, false)
	metrics.GuestCall(0, "plugin_service_get_info", time.Millisecond, "ok")
	metrics.OnPluginDisabled("transient", 0, "guest module exited")

	plugin := PluginLabel(42)

	assert.InDelta(t, 1, testutil.ToFloat64(metrics.hostCalls.WithLabelValues(plugin, "nodecmd", "execute_command", "ok")), 0)
	assert.InDelta(t, 1, testutil.ToFloat64(metrics.hostCalls.WithLabelValues(plugin, "nodecmd", "execute_command", "panic")), 0)
	assert.InDelta(t, 1, testutil.ToFloat64(metrics.hostCallsDenied.WithLabelValues(plugin, "nodecmd", "execute_command", "permission")), 0)
	assert.InDelta(t, 1, testutil.ToFloat64(metrics.guestCalls.WithLabelValues(plugin, "plugin_service_handle_event", "ok")), 0)
	assert.InDelta(t, 1, testutil.ToFloat64(metrics.guestCalls.WithLabelValues(plugin, "plugin_service_handle_event", "timeout")), 0)
	assert.InDelta(t, 1, testutil.ToFloat64(metrics.events.WithLabelValues("SERVER_POST_START", "handled")), 0)
	assert.InDelta(t, 1, testutil.ToFloat64(metrics.disabled.WithLabelValues(plugin, "event handler timed out")), 0,
		"the reason label keeps only the stable prefix")
	assert.InDelta(t, 3, testutil.ToFloat64(metrics.backlog), 0)

	assert.InDelta(t, 0, testutil.ToFloat64(metrics.hostCalls.WithLabelValues("0", "log", "log", "ok")), 0)

	rec := httptest.NewRecorder()
	registry.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()

	for _, name := range []string{
		"gameap_plugin_host_calls_total", "gameap_plugin_host_call_duration_seconds",
		"gameap_plugin_host_calls_denied_total", "gameap_plugin_guest_calls_total",
		"gameap_plugin_guest_call_duration_seconds", "gameap_plugin_events_dispatched_total",
		"gameap_plugin_disabled_total", "gameap_plugin_async_backlog",
		"go_goroutines", "process_cpu_seconds_total",
	} {
		assert.Truef(t, strings.Contains(body, name), "exposition must contain %s", name)
	}
}

func TestPluginMetrics_without_sources_collects_nothing_per_plugin(t *testing.T) {
	t.Parallel()

	registry := New()
	NewPluginMetrics(registry, nil, nil, nil)

	families, err := registry.Gather().Gather()
	require.NoError(t, err)

	for _, family := range families {
		assert.NotEqual(t, "gameap_plugin_memory_bytes", family.GetName())
		assert.NotEqual(t, "gameap_plugin_enabled", family.GetName())
	}
}

type stubPluginLister struct{ plugins []*pkgplugin.LoadedPlugin }

func (s stubPluginLister) GetPlugins() []*pkgplugin.LoadedPlugin { return s.plugins }

func TestPluginMetrics_exposes_self_reported_health(t *testing.T) {
	t.Parallel()

	reporting := &pkgplugin.LoadedPlugin{Info: &proto.PluginInfo{Id: "reporting"}, Enabled: true, DBID: 42}
	reporting.SetHealth(pkgplugin.HealthReport{Status: pkgplugin.HealthDegraded, Message: "steam api unreachable"})
	silent := &pkgplugin.LoadedPlugin{Info: &proto.PluginInfo{Id: "silent"}, Enabled: true, DBID: 43}

	registry := New()
	NewPluginMetrics(registry, stubPluginLister{plugins: []*pkgplugin.LoadedPlugin{reporting, silent}}, stubBacklog{}, nil)

	rec := httptest.NewRecorder()
	registry.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, `gameap_plugin_health{plugin="`+PluginLabel(42)+`",status="degraded"} 1`)
	assert.Contains(t, body, `gameap_plugin_health{plugin="`+PluginLabel(42)+`",status="healthy"} 0`)
	assert.Contains(t, body, `gameap_plugin_health{plugin="`+PluginLabel(42)+`",status="unhealthy"} 0`)
	assert.NotContains(t, body, `gameap_plugin_health{plugin="`+PluginLabel(43)+`"`, "no report, no series")
}
