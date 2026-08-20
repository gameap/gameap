package metrics

import (
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func makePoint(ts time.Time, value int64) *proto.MetricPoint {
	return &proto.MetricPoint{
		Timestamp: timestamppb.New(ts),
		Value:     &proto.MetricPoint_IntValue{IntValue: value},
	}
}

func makeSeries(name string, labels map[string]string, points ...*proto.MetricPoint) *proto.MetricSeries {
	return &proto.MetricSeries{
		Name:   name,
		Type:   proto.MetricType_METRIC_TYPE_GAUGE,
		Unit:   proto.MetricUnit_METRIC_UNIT_PERCENT,
		Labels: labels,
		Points: points,
	}
}

func makeResponse(ts time.Time, series ...*proto.MetricSeries) *proto.MetricsResponse {
	return &proto.MetricsResponse{
		Timestamp: timestamppb.New(ts),
		Series:    series,
	}
}

func TestMergeResponses_EmptyInput_ReturnsNil(t *testing.T) {
	t.Parallel()
	assert.Nil(t, mergeResponses(nil))
	assert.Nil(t, mergeResponses([]*proto.MetricsResponse{}))
}

func TestMergeResponses_SingleEntry_ReturnsInputVerbatim(t *testing.T) {
	t.Parallel()
	now := time.Now()
	in := makeResponse(now, makeSeries("cpu", nil, makePoint(now, 1)))

	out := mergeResponses([]*proto.MetricsResponse{in})

	assert.Same(t, in, out)
}

func TestMergeResponses_TwoEntriesSameSeries_PointsConcatenatedAndSorted(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(5 * time.Second)
	t2 := t0.Add(10 * time.Second)

	entries := []*proto.MetricsResponse{
		makeResponse(t0, makeSeries("cpu", nil, makePoint(t0, 1))),
		makeResponse(t1, makeSeries("cpu", nil, makePoint(t1, 2))),
		makeResponse(t2, makeSeries("cpu", nil, makePoint(t2, 3))),
	}

	out := mergeResponses(entries)
	require.NotNil(t, out)
	require.Len(t, out.Series, 1)
	require.Len(t, out.Series[0].Points, 3)

	assert.Equal(t, t0, out.Series[0].Points[0].GetTimestamp().AsTime())
	assert.Equal(t, t1, out.Series[0].Points[1].GetTimestamp().AsTime())
	assert.Equal(t, t2, out.Series[0].Points[2].GetTimestamp().AsTime())
	assert.Equal(t, t2, out.GetTimestamp().AsTime())
}

func TestMergeResponses_DifferentLabelCombinations_StaySeparate(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(5 * time.Second)

	entries := []*proto.MetricsResponse{
		makeResponse(t0,
			makeSeries("disk", map[string]string{"mount": "/"}, makePoint(t0, 100)),
			makeSeries("disk", map[string]string{"mount": "/var"}, makePoint(t0, 200)),
		),
		makeResponse(t1,
			makeSeries("disk", map[string]string{"mount": "/"}, makePoint(t1, 110)),
			makeSeries("disk", map[string]string{"mount": "/var"}, makePoint(t1, 210)),
		),
	}

	out := mergeResponses(entries)
	require.Len(t, out.Series, 2)

	for _, s := range out.Series {
		require.Len(t, s.Points, 2, "series %s should have 2 points", s.GetLabels()["mount"])
	}
}

func TestMergeResponses_NodeAndServerSeries_GroupedPerIdentity(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(5 * time.Second)

	entries := []*proto.MetricsResponse{
		makeResponse(t0,
			makeSeries("node_cpu", nil, makePoint(t0, 1)),
			makeSeries("server_mem", map[string]string{"server_id": "7"}, makePoint(t0, 1024)),
		),
		makeResponse(t1,
			makeSeries("node_cpu", nil, makePoint(t1, 2)),
			makeSeries("server_mem", map[string]string{"server_id": "7"}, makePoint(t1, 2048)),
		),
	}

	out := mergeResponses(entries)
	require.Len(t, out.Series, 2)

	byName := map[string]*proto.MetricSeries{}
	for _, s := range out.Series {
		byName[s.GetName()] = s
	}
	require.Len(t, byName["node_cpu"].Points, 2)
	require.Len(t, byName["server_mem"].Points, 2)
}

func TestMergeResponses_DuplicateTimestamps_Deduped(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(5 * time.Second)

	entries := []*proto.MetricsResponse{
		makeResponse(t0,
			makeSeries("cpu", nil, makePoint(t0, 1), makePoint(t1, 2)),
		),
		makeResponse(t1,
			makeSeries("cpu", nil, makePoint(t1, 99)),
		),
	}

	out := mergeResponses(entries)
	require.Len(t, out.Series, 1)
	require.Len(t, out.Series[0].Points, 2)

	assert.Equal(t, int64(2), out.Series[0].Points[1].GetIntValue(),
		"first occurrence of duplicate timestamp wins")
}

func TestMergeResponses_OutOfOrderEntries_PointsSortedByTimestamp(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(5 * time.Second)
	t2 := t0.Add(10 * time.Second)

	entries := []*proto.MetricsResponse{
		makeResponse(t2, makeSeries("cpu", nil, makePoint(t2, 3))),
		makeResponse(t0, makeSeries("cpu", nil, makePoint(t0, 1))),
		makeResponse(t1, makeSeries("cpu", nil, makePoint(t1, 2))),
	}

	out := mergeResponses(entries)
	require.NotNil(t, out)
	require.Len(t, out.Series[0].Points, 3)

	assert.Equal(t, t0, out.Series[0].Points[0].GetTimestamp().AsTime())
	assert.Equal(t, t1, out.Series[0].Points[1].GetTimestamp().AsTime())
	assert.Equal(t, t2, out.Series[0].Points[2].GetTimestamp().AsTime())
}

func TestMergeResponses_RealWorldShape_ReducesEnvelopesToOne(t *testing.T) {
	t.Parallel()
	const ticks = 360
	const seriesPerTick = 31

	base := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	entries := make([]*proto.MetricsResponse, 0, ticks)
	for i := range ticks {
		ts := base.Add(time.Duration(i) * 5 * time.Second)
		series := make([]*proto.MetricSeries, 0, seriesPerTick)
		for j := range seriesPerTick {
			labels := map[string]string{"id": stringFromInt(j)}
			series = append(series, makeSeries("metric", labels, makePoint(ts, int64(i))))
		}
		entries = append(entries, makeResponse(ts, series...))
	}

	out := mergeResponses(entries)
	require.NotNil(t, out)
	require.Len(t, out.Series, seriesPerTick)
	for _, s := range out.Series {
		require.Len(t, s.Points, ticks)
	}
}

func TestMergeResponses_PreservesActualWindowSecondsMax(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(5 * time.Second)

	entries := []*proto.MetricsResponse{
		{Timestamp: timestamppb.New(t0), ActualWindowSeconds: 0},
		{Timestamp: timestamppb.New(t1), ActualWindowSeconds: 1800},
	}

	out := mergeResponses(entries)
	require.NotNil(t, out)
	assert.Equal(t, uint32(1800), out.GetActualWindowSeconds())
	assert.Equal(t, t1, out.GetTimestamp().AsTime())
}

func TestSeriesIdentityKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		series *proto.MetricSeries
		want   string
	}{
		{
			name:   "no_labels",
			series: makeSeries("cpu", nil),
			want:   "cpu",
		},
		{
			name:   "empty_labels_map",
			series: makeSeries("cpu", map[string]string{}),
			want:   "cpu",
		},
		{
			name:   "single_label",
			series: makeSeries("disk", map[string]string{"mount": "/"}),
			want:   "disk|mount=/",
		},
		{
			name:   "multiple_labels__sorted",
			series: makeSeries("net", map[string]string{"iface": "eth0", "dir": "rx"}),
			want:   "net|dir=rx|iface=eth0",
		},
		{
			name:   "labels_in_reverse_order__same_key",
			series: makeSeries("net", map[string]string{"iface": "eth0", "dir": "rx"}),
			want:   "net|dir=rx|iface=eth0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, seriesIdentityKey(tc.series))
		})
	}
}

func stringFromInt(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}

	return string(rune('a' + (i - 10)))
}
