package pluginssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCapturedStream_Write pins both halves of the contract: the head of the
// stream is kept up to the cap, and every write is accepted whole. A short
// write or an error would stop the SSH reader from draining the channel and
// hang the remote command, so the invariant is asserted on every single write.
func TestCapturedStream_Write(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		capacity      int
		writes        []string
		wantCaptured  string
		wantTruncated bool
		wantTotal     uint64
	}{
		{
			name:         "a_write_below_the_capacity_is_kept_whole",
			capacity:     16,
			writes:       []string{"hello"},
			wantCaptured: "hello",
			wantTotal:    5,
		},
		{
			name:         "a_write_exactly_at_the_capacity_is_not_truncated",
			capacity:     5,
			writes:       []string{"hello"},
			wantCaptured: "hello",
			wantTotal:    5,
		},
		{
			name:          "a_write_above_the_capacity_keeps_only_the_head",
			capacity:      4,
			writes:        []string{"hello"},
			wantCaptured:  "hell",
			wantTruncated: true,
			wantTotal:     5,
		},
		{
			name:         "consecutive_writes_accumulate_up_to_the_capacity",
			capacity:     8,
			writes:       []string{"ab", "cd", "ef"},
			wantCaptured: "abcdef",
			wantTotal:    6,
		},
		{
			name:          "a_write_crossing_the_capacity_is_split_at_the_boundary",
			capacity:      6,
			writes:        []string{"abcd", "efgh"},
			wantCaptured:  "abcdef",
			wantTruncated: true,
			wantTotal:     8,
		},
		{
			name:          "a_write_into_a_full_buffer_is_dropped_but_still_counted",
			capacity:      5,
			writes:        []string{"hello", "world"},
			wantCaptured:  "hello",
			wantTruncated: true,
			wantTotal:     10,
		},
		{
			name:         "an_empty_write_into_a_full_buffer_leaves_the_flag_down",
			capacity:     5,
			writes:       []string{"hello", ""},
			wantCaptured: "hello",
			wantTotal:    5,
		},
		{
			name:          "a_zero_capacity_buffer_captures_nothing_but_reports_the_size",
			capacity:      0,
			writes:        []string{"x"},
			wantCaptured:  "",
			wantTruncated: true,
			wantTotal:     1,
		},
		{
			name:         "an_empty_write_into_a_zero_capacity_buffer_leaves_the_flag_down",
			capacity:     0,
			writes:       []string{""},
			wantCaptured: "",
			wantTotal:    0,
		},
		{
			name:         "a_stream_nobody_wrote_to_reports_nothing",
			capacity:     8,
			writes:       nil,
			wantCaptured: "",
			wantTotal:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			stream := newCapturedStream(tt.capacity)

			// ACT
			accepted := make([]int, 0, len(tt.writes))
			failures := make([]error, 0, len(tt.writes))
			for _, write := range tt.writes {
				n, err := stream.Write([]byte(write))
				accepted = append(accepted, n)
				failures = append(failures, err)
			}

			chunk, next, truncated, total := stream.slice(0)

			// ASSERT
			for i, err := range failures {
				require.NoError(t, err, "write %d: Write must never fail, the SSH reader would stop draining", i)
				assert.Equal(t, len(tt.writes[i]), accepted[i],
					"write %d: a short write stalls the channel window and hangs the remote command", i)
			}

			assert.Equal(t, tt.wantCaptured, string(chunk), "only the head of the stream is retained")
			assert.Equal(t, tt.wantTruncated, truncated, "the flag tells the plugin whether it is seeing everything")
			assert.Equal(t, tt.wantTotal, total, "the total counts what the command produced, captured or dropped")
			assert.Equal(t, uint64(len(tt.wantCaptured)), next, "the next offset is the captured length")
		})
	}
}

// TestCapturedStream_Slice covers the offsets a plugin polls output with: it
// asks for everything after what it already read, and an offset that ran past
// the capture must come back empty instead of panicking.
func TestCapturedStream_Slice(t *testing.T) {
	t.Parallel()

	const captured = "abcdef"

	tests := []struct {
		name      string
		offset    uint64
		wantChunk string
	}{
		{name: "a_zero_offset_returns_everything_captured", offset: 0, wantChunk: captured},
		{name: "an_offset_in_the_middle_returns_the_tail", offset: 3, wantChunk: "def"},
		{name: "an_offset_at_the_end_returns_an_empty_chunk", offset: 6, wantChunk: ""},
		{name: "an_offset_past_the_end_is_clamped_to_the_end", offset: 99, wantChunk: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			stream := newCapturedStream(16)
			_, err := stream.Write([]byte(captured))
			require.NoError(t, err)

			// ACT
			chunk, next, truncated, total := stream.slice(tt.offset)

			// ASSERT
			assert.Equal(t, tt.wantChunk, string(chunk))
			assert.Equal(t, uint64(len(captured)), next,
				"the next offset is always the captured length, so a plugin that overshot catches up")
			assert.False(t, truncated)
			assert.Equal(t, uint64(len(captured)), total)
		})
	}
}

// TestCapturedStream_SliceReturnsACopy: the chunk travels into guest memory,
// so it must not alias the capture buffer that the SSH reader keeps writing to.
func TestCapturedStream_SliceReturnsACopy(t *testing.T) {
	t.Parallel()
	// ARRANGE
	stream := newCapturedStream(16)
	_, err := stream.Write([]byte("secret"))
	require.NoError(t, err)

	chunk, next, truncated, total := stream.slice(0)
	require.Equal(t, "secret", string(chunk))
	require.Equal(t, uint64(6), next)
	require.False(t, truncated)
	require.Equal(t, uint64(6), total)

	// ACT
	chunk[0] = 'S'

	// ASSERT
	after, _, _, _ := stream.slice(0) //nolint:dogsled // only the captured bytes matter here
	assert.Equal(t, "secret", string(after), "a chunk handed out must not share memory with the capture buffer")
}
