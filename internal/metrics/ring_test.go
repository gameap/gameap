package metrics

import (
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func newEntry(ts time.Time) *proto.MetricsResponse {
	return &proto.MetricsResponse{
		Timestamp: timestamppb.New(ts),
	}
}

func TestRing_Append_BoundsCapacity(t *testing.T) {
	t.Parallel()
	r := newRing(3)

	base := time.Now()
	for i := range 5 {
		r.Append(newEntry(base.Add(time.Duration(i) * time.Second)))
	}

	require.Equal(t, 3, r.Len())

	snap := r.Snapshot(time.Time{})
	require.Len(t, snap, 3)
	assert.Equal(t, base.Add(2*time.Second).Unix(), snap[0].GetTimestamp().AsTime().Unix())
	assert.Equal(t, base.Add(4*time.Second).Unix(), snap[2].GetTimestamp().AsTime().Unix())
}

func TestRing_Snapshot_AgeCutoff(t *testing.T) {
	t.Parallel()
	r := newRing(10)

	now := time.Now()
	r.Append(newEntry(now.Add(-30 * time.Second)))
	r.Append(newEntry(now.Add(-20 * time.Second)))
	r.Append(newEntry(now.Add(-10 * time.Second)))

	snap := r.Snapshot(now.Add(-15 * time.Second))
	require.Len(t, snap, 1)
	assert.Equal(t, now.Add(-10*time.Second).Unix(), snap[0].GetTimestamp().AsTime().Unix())
}

func TestRing_Snapshot_AllNewer(t *testing.T) {
	t.Parallel()
	r := newRing(10)

	now := time.Now()
	r.Append(newEntry(now))
	r.Append(newEntry(now.Add(time.Second)))

	snap := r.Snapshot(now.Add(-time.Hour))
	require.Len(t, snap, 2)
}

func TestRing_Snapshot_AllOlder_ReturnsNil(t *testing.T) {
	t.Parallel()
	r := newRing(10)

	now := time.Now()
	r.Append(newEntry(now.Add(-time.Hour)))

	snap := r.Snapshot(now)
	assert.Nil(t, snap)
}

func TestRing_Newest(t *testing.T) {
	t.Parallel()
	r := newRing(3)

	assert.Nil(t, r.Newest())

	now := time.Now()
	r.Append(newEntry(now))
	r.Append(newEntry(now.Add(time.Second)))

	newest := r.Newest()
	require.NotNil(t, newest)
	assert.Equal(t, now.Add(time.Second).Unix(), newest.GetTimestamp().AsTime().Unix())
}

func TestRing_Empty(t *testing.T) {
	t.Parallel()
	r := newRing(5)

	assert.Equal(t, 0, r.Len())
	assert.Nil(t, r.Snapshot(time.Time{}))
	assert.Nil(t, r.Newest())
}

func TestRing_AppendNil_NoOp(t *testing.T) {
	t.Parallel()
	r := newRing(3)
	r.Append(nil)
	assert.Equal(t, 0, r.Len())
}

func TestRing_ConcurrentReadWrite(t *testing.T) {
	t.Parallel()
	r := newRing(50)

	var wg sync.WaitGroup
	now := time.Now()

	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 200 {
			r.Append(newEntry(now.Add(time.Duration(i) * time.Millisecond)))
		}
	}()

	go func() {
		defer wg.Done()
		for range 200 {
			_ = r.Snapshot(time.Time{})
			_ = r.Newest()
			_ = r.Len()
		}
	}()

	wg.Wait()

	assert.LessOrEqual(t, r.Len(), 50)
	assert.Greater(t, r.Len(), 0)
}

func TestRing_MinCapacity(t *testing.T) {
	t.Parallel()
	r := newRing(0)
	assert.Equal(t, 1, r.capacity)

	r.Append(newEntry(time.Now()))
	r.Append(newEntry(time.Now().Add(time.Second)))

	assert.Equal(t, 1, r.Len())
}
