package metrics

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func makeDoublePoint(ts time.Time, value float64) *proto.MetricPoint {
	return &proto.MetricPoint{
		Timestamp: timestamppb.New(ts),
		Value:     &proto.MetricPoint_DoubleValue{DoubleValue: value},
	}
}

func makeUintPoint(ts time.Time, value uint64) *proto.MetricPoint {
	return &proto.MetricPoint{
		Timestamp: timestamppb.New(ts),
		Value:     &proto.MetricPoint_UintValue{UintValue: value},
	}
}

func makePointWithStats(ts time.Time, value int64, minV, maxV, avgV *float64, sampleCount *uint32) *proto.MetricPoint {
	return &proto.MetricPoint{
		Timestamp:   timestamppb.New(ts),
		Value:       &proto.MetricPoint_IntValue{IntValue: value},
		Min:         minV,
		Max:         maxV,
		Avg:         avgV,
		SampleCount: sampleCount,
	}
}

func TestToWire_NilResponse_ReturnsNil(t *testing.T) {
	t.Parallel()
	// ARRANGE / ACT
	got := ToWire(nil, nil)

	// ASSERT
	assert.Nil(t, got)
}

func TestToWire_NilSeries_ReturnsNil(t *testing.T) {
	t.Parallel()
	// ARRANGE
	resp := &proto.MetricsResponse{}

	// ACT
	got := ToWire(resp, nil)

	// ASSERT
	assert.Nil(t, got, "empty Series must collapse to nil instead of an empty wire envelope")
}

func TestToWire_NilFilter_KeepsAllSeries(t *testing.T) {
	t.Parallel()
	// ARRANGE
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	resp := makeResponse(now,
		makeSeries("cpu", nil, makePoint(now, 1)),
		makeSeries("mem", map[string]string{"host": "h1"}, makePoint(now, 2)),
	)

	// ACT
	got := ToWire(resp, nil)

	// ASSERT
	require.NotNil(t, got)
	require.Len(t, got.Series, 2)
	assert.Equal(t, "cpu", got.Series[0].Name)
	assert.Equal(t, "mem", got.Series[1].Name)
}

func TestToWire_FilterDropsAll_ReturnsNil(t *testing.T) {
	t.Parallel()
	// ARRANGE
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	resp := makeResponse(now,
		makeSeries("cpu", nil, makePoint(now, 1)),
		makeSeries("mem", nil, makePoint(now, 2)),
	)

	// ACT
	got := ToWire(resp, func(*proto.MetricSeries) bool { return false })

	// ASSERT
	assert.Nil(t, got, "all-series-filtered-out must collapse the envelope")
}

func TestToWire_FilterSelective_KeepsOnlyMatching(t *testing.T) {
	t.Parallel()
	// ARRANGE
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	resp := makeResponse(now,
		makeSeries("cpu", nil, makePoint(now, 1)),
		makeSeries("mem", nil, makePoint(now, 2)),
		makeSeries("disk", nil, makePoint(now, 3)),
	)

	// ACT
	got := ToWire(resp, func(s *proto.MetricSeries) bool {
		return s.GetName() == "mem"
	})

	// ASSERT
	require.NotNil(t, got)
	require.Len(t, got.Series, 1)
	assert.Equal(t, "mem", got.Series[0].Name)
}

func TestToWire_PreservesTimestampAndCommonLabelsAndWindow(t *testing.T) {
	t.Parallel()
	// ARRANGE
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	resp := makeResponse(now, makeSeries("cpu", nil, makePoint(now, 1)))
	resp.CommonLabels = map[string]string{"node": "n7", "region": "eu"}
	resp.ActualWindowSeconds = 600

	// ACT
	got := ToWire(resp, nil)

	// ASSERT
	require.NotNil(t, got)
	assert.Equal(t, now, got.Timestamp)
	assert.Equal(t, map[string]string{"node": "n7", "region": "eu"}, got.CommonLabels)
	assert.Equal(t, uint32(600), got.ActualWindowSeconds)
}

func TestToWire_PreservesSeriesShape(t *testing.T) {
	t.Parallel()
	// ARRANGE
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	t1 := now.Add(5 * time.Second)
	t2 := now.Add(10 * time.Second)

	labelsA := map[string]string{"iface": "eth0"}
	labelsB := map[string]string{"iface": "eth1"}

	resp := makeResponse(now,
		makeSeries("net_rx", labelsA, makePoint(now, 1), makePoint(t1, 2), makePoint(t2, 3)),
		makeSeries("net_rx", labelsB, makePoint(now, 10), makePoint(t1, 20), makePoint(t2, 30)),
	)

	// ACT
	got := ToWire(resp, nil)

	// ASSERT
	require.NotNil(t, got)
	require.Len(t, got.Series, 2)
	for i, s := range got.Series {
		assert.Equal(t, "net_rx", s.Name, "series %d name", i)
		assert.Equal(t, "gauge", s.Type, "series %d type", i)
		assert.Equal(t, "percent", s.Unit, "series %d unit", i)
		require.Len(t, s.Points, 3, "series %d points", i)
	}
	assert.Equal(t, labelsA, got.Series[0].Labels)
	assert.Equal(t, labelsB, got.Series[1].Labels)
}

func TestToWire_AppliesEnumMappingsToSeries(t *testing.T) {
	t.Parallel()
	// ARRANGE
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	series := &proto.MetricSeries{
		Name:   "bytes_read",
		Type:   proto.MetricType_METRIC_TYPE_COUNTER,
		Unit:   proto.MetricUnit_METRIC_UNIT_BYTES,
		Points: []*proto.MetricPoint{makePoint(now, 1024)},
	}
	resp := makeResponse(now, series)

	// ACT
	got := ToWire(resp, nil)

	// ASSERT
	require.NotNil(t, got)
	require.Len(t, got.Series, 1)
	assert.Equal(t, "counter", got.Series[0].Type)
	assert.Equal(t, "bytes", got.Series[0].Unit)
}

func TestEncodePoint_AllStatsPresent_AllCopied(t *testing.T) {
	t.Parallel()
	// ARRANGE
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	srcMin := 1.5
	srcMax := 9.75
	srcAvg := 4.25
	srcCount := uint32(42)
	src := makePointWithStats(now, 10, &srcMin, &srcMax, &srcAvg, &srcCount)

	// ACT
	wp := encodePoint(src)

	// ASSERT
	require.NotNil(t, wp.Min)
	require.NotNil(t, wp.Max)
	require.NotNil(t, wp.Avg)
	require.NotNil(t, wp.SampleCount)

	assert.Equal(t, srcMin, *wp.Min)
	assert.Equal(t, srcMax, *wp.Max)
	assert.Equal(t, srcAvg, *wp.Avg)
	assert.Equal(t, srcCount, *wp.SampleCount)

	// Wire pointers must be independent copies so callers can't mutate the proto via the wire struct.
	assert.NotSame(t, &srcMin, wp.Min, "Min must be a fresh pointer")
	assert.NotSame(t, &srcMax, wp.Max, "Max must be a fresh pointer")
	assert.NotSame(t, &srcAvg, wp.Avg, "Avg must be a fresh pointer")
	assert.NotSame(t, &srcCount, wp.SampleCount, "SampleCount must be a fresh pointer")

	assert.Equal(t, now, wp.Timestamp)
	assert.Equal(t, "10", wp.Value)
}

func TestEncodePoint_NoStats_AllNil(t *testing.T) {
	t.Parallel()
	// ARRANGE
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	src := makePoint(now, 5)

	// ACT
	wp := encodePoint(src)

	// ASSERT
	assert.Nil(t, wp.Min)
	assert.Nil(t, wp.Max)
	assert.Nil(t, wp.Avg)
	assert.Nil(t, wp.SampleCount)
	assert.Equal(t, now, wp.Timestamp)
	assert.Equal(t, "5", wp.Value)
}

func TestEncodePoint_PartialStats_OnlyPresentCopied(t *testing.T) {
	t.Parallel()
	// ARRANGE
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	srcMin := -2.5
	srcAvg := 0.5
	src := makePointWithStats(now, 7, &srcMin, nil, &srcAvg, nil)

	// ACT
	wp := encodePoint(src)

	// ASSERT
	require.NotNil(t, wp.Min)
	require.NotNil(t, wp.Avg)
	assert.Equal(t, srcMin, *wp.Min)
	assert.Equal(t, srcAvg, *wp.Avg)
	assert.Nil(t, wp.Max)
	assert.Nil(t, wp.SampleCount)
}

func TestPointValue(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		point *proto.MetricPoint
		want  string
	}{
		{
			name:  "double_positive",
			point: makeDoublePoint(now, 1.5),
			want:  "1.5",
		},
		{
			name:  "double_negative",
			point: makeDoublePoint(now, -2.25),
			want:  "-2.25",
		},
		{
			name:  "double_zero",
			point: makeDoublePoint(now, 0),
			want:  "0",
		},
		{
			name:  "double_integer_value",
			point: makeDoublePoint(now, 3.0),
			want:  "3",
		},
		{
			name:  "double_very_small",
			point: makeDoublePoint(now, 1e-12),
			want:  "1e-12",
		},
		{
			name:  "double_very_large",
			point: makeDoublePoint(now, 1e20),
			want:  "1e+20",
		},
		{
			name:  "uint_zero",
			point: makeUintPoint(now, 0),
			want:  "0",
		},
		{
			name:  "uint_value",
			point: makeUintPoint(now, 12345),
			want:  "12345",
		},
		{
			name:  "uint_max",
			point: makeUintPoint(now, math.MaxUint64),
			want:  "18446744073709551615",
		},
		{
			name:  "int_positive",
			point: makePoint(now, 42),
			want:  "42",
		},
		{
			name:  "int_negative",
			point: makePoint(now, -7),
			want:  "-7",
		},
		{
			name:  "int_zero",
			point: makePoint(now, 0),
			want:  "0",
		},
		{
			name:  "int_min",
			point: makePoint(now, math.MinInt64),
			want:  "-9223372036854775808",
		},
		{
			name:  "nil_oneof_value",
			point: &proto.MetricPoint{},
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			got := pointValue(tc.point)

			// ASSERT
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMetricTypeName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input proto.MetricType
		want  string
	}{
		{
			name:  "gauge",
			input: proto.MetricType_METRIC_TYPE_GAUGE,
			want:  "gauge",
		},
		{
			name:  "counter",
			input: proto.MetricType_METRIC_TYPE_COUNTER,
			want:  "counter",
		},
		{
			name:  "unspecified_explicit",
			input: proto.MetricType_METRIC_TYPE_UNSPECIFIED,
			want:  "unspecified",
		},
		{
			name:  "unknown_value",
			input: proto.MetricType(99),
			want:  "unspecified",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			got := metricTypeName(tc.input)

			// ASSERT
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMetricUnitName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input proto.MetricUnit
		want  string
	}{
		{name: "count", input: proto.MetricUnit_METRIC_UNIT_COUNT, want: "count"},
		{name: "percent", input: proto.MetricUnit_METRIC_UNIT_PERCENT, want: "percent"},
		{name: "ratio", input: proto.MetricUnit_METRIC_UNIT_RATIO, want: "ratio"},
		{name: "bytes", input: proto.MetricUnit_METRIC_UNIT_BYTES, want: "bytes"},
		{name: "bits", input: proto.MetricUnit_METRIC_UNIT_BITS, want: "bits"},
		{name: "seconds", input: proto.MetricUnit_METRIC_UNIT_SECONDS, want: "seconds"},
		{name: "milliseconds", input: proto.MetricUnit_METRIC_UNIT_MILLISECONDS, want: "milliseconds"},
		{name: "microseconds", input: proto.MetricUnit_METRIC_UNIT_MICROSECONDS, want: "microseconds"},
		{name: "nanoseconds", input: proto.MetricUnit_METRIC_UNIT_NANOSECONDS, want: "nanoseconds"},
		{name: "hertz", input: proto.MetricUnit_METRIC_UNIT_HERTZ, want: "hertz"},
		{name: "celsius", input: proto.MetricUnit_METRIC_UNIT_CELSIUS, want: "celsius"},
		{name: "watts", input: proto.MetricUnit_METRIC_UNIT_WATTS, want: "watts"},
		{name: "volts", input: proto.MetricUnit_METRIC_UNIT_VOLTS, want: "volts"},
		{name: "rpm", input: proto.MetricUnit_METRIC_UNIT_RPM, want: "rpm"},
		{name: "unspecified_explicit", input: proto.MetricUnit_METRIC_UNIT_UNSPECIFIED, want: "unspecified"},
		{name: "unknown_value", input: proto.MetricUnit(999), want: "unspecified"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			got := metricUnitName(tc.input)

			// ASSERT
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestToWire_JSONMarshal_OmitemptyTagsRespected(t *testing.T) {
	t.Parallel()
	// ARRANGE
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	resp := &proto.MetricsResponse{
		Timestamp: timestamppb.New(now),
		Series: []*proto.MetricSeries{
			{
				Name: "cpu",
				Type: proto.MetricType_METRIC_TYPE_GAUGE,
				Unit: proto.MetricUnit_METRIC_UNIT_PERCENT,
				Points: []*proto.MetricPoint{
					makePoint(now, 1),
				},
			},
		},
	}

	// ACT
	wire := ToWire(resp, nil)
	require.NotNil(t, wire)

	data, err := json.Marshal(wire)
	require.NoError(t, err)

	// ASSERT
	got := string(data)

	for _, key := range []string{"common_labels", "actual_window_seconds", "labels", "min", "max", "avg", "sample_count"} {
		assert.NotContains(t, got, "\""+key+"\"", "omitempty key %q must be absent when unset", key)
	}
	for _, key := range []string{"timestamp", "series", "name", "type", "unit", "points", "value"} {
		assert.Contains(t, got, "\""+key+"\"", "key %q must be present", key)
	}
}

func TestToWire_JSONMarshal_PopulatedFields_AllPresent(t *testing.T) {
	t.Parallel()
	// ARRANGE
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	srcMin := 1.0
	srcMax := 9.5
	srcAvg := 4.2
	srcCount := uint32(7)

	resp := &proto.MetricsResponse{
		Timestamp:           timestamppb.New(now),
		CommonLabels:        map[string]string{"region": "eu"},
		ActualWindowSeconds: 300,
		Series: []*proto.MetricSeries{
			{
				Name:   "load",
				Type:   proto.MetricType_METRIC_TYPE_GAUGE,
				Unit:   proto.MetricUnit_METRIC_UNIT_RATIO,
				Labels: map[string]string{"core": "0"},
				Points: []*proto.MetricPoint{
					makePointWithStats(now, 4, &srcMin, &srcMax, &srcAvg, &srcCount),
				},
			},
		},
	}

	// ACT
	wire := ToWire(resp, nil)
	require.NotNil(t, wire)

	data, err := json.Marshal(wire)
	require.NoError(t, err)

	// ASSERT
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Contains(t, decoded, "common_labels")
	assert.Contains(t, decoded, "actual_window_seconds")
	assert.EqualValues(t, 300, decoded["actual_window_seconds"])

	seriesAny, ok := decoded["series"].([]any)
	require.True(t, ok, "series must be a JSON array")
	require.Len(t, seriesAny, 1)

	s0, ok := seriesAny[0].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, s0, "labels")

	pointsAny, ok := s0["points"].([]any)
	require.True(t, ok)
	require.Len(t, pointsAny, 1)

	p0, ok := pointsAny[0].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, p0, "min")
	assert.Contains(t, p0, "max")
	assert.Contains(t, p0, "avg")
	assert.Contains(t, p0, "sample_count")
	assert.EqualValues(t, srcMin, p0["min"])
	assert.EqualValues(t, srcMax, p0["max"])
	assert.EqualValues(t, srcAvg, p0["avg"])
	assert.EqualValues(t, srcCount, p0["sample_count"])
}
