package metricsbase

import (
	"testing"

	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
)

func TestNodePrefixFilter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		series *proto.MetricSeries
		want   bool
	}{
		{
			name:   "node_prefix__pass",
			series: &proto.MetricSeries{Name: "gameap_node_cpu_usage_percent"},
			want:   true,
		},
		{
			name:   "node_prefix_with_labels__pass",
			series: &proto.MetricSeries{Name: "gameap_node_disk_usage_percent", Labels: map[string]string{"mount": "/"}},
			want:   true,
		},
		{
			name:   "server_prefix__drop",
			series: &proto.MetricSeries{Name: "gameap_server_memory_limit_bytes", Labels: map[string]string{"server_id": "7"}},
			want:   false,
		},
		{
			name:   "server_process_pids__drop",
			series: &proto.MetricSeries{Name: "gameap_server_process_pids", Labels: map[string]string{"server_id": "7"}},
			want:   false,
		},
		{
			name:   "empty_name__drop",
			series: &proto.MetricSeries{Name: ""},
			want:   false,
		},
		{
			name:   "missing_underscore_after_node__drop",
			series: &proto.MetricSeries{Name: "gameap_nodefoo"},
			want:   false,
		},
		{
			name:   "unrelated_prefix__drop",
			series: &proto.MetricSeries{Name: "gameap_daemon_uptime_seconds_total"},
			want:   false,
		},
	}

	filter := NodePrefixFilter()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, filter(tc.series))
		})
	}
}
