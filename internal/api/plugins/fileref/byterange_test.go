package fileref

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseByteRange(t *testing.T) {
	t.Parallel()

	const size = 100

	tests := []struct {
		name            string
		header          string
		wantOK          bool
		wantSatisfiable bool
		wantStart       uint64
		wantLength      uint64
	}{
		{name: "absent", header: "", wantOK: false},
		{name: "other_unit", header: "items=0-10", wantOK: false},
		{name: "closed", header: "bytes=0-9", wantOK: true, wantSatisfiable: true, wantStart: 0, wantLength: 10},
		{name: "open_ended", header: "bytes=90-", wantOK: true, wantSatisfiable: true, wantStart: 90, wantLength: 10},
		{name: "suffix", header: "bytes=-10", wantOK: true, wantSatisfiable: true, wantStart: 90, wantLength: 10},
		{
			name:   "suffix_longer_than_the_file_is_the_whole_file",
			header: "bytes=-500", wantOK: true, wantSatisfiable: true, wantStart: 0, wantLength: size,
		},
		{
			name:   "end_past_the_file_is_clamped",
			header: "bytes=95-500", wantOK: true, wantSatisfiable: true, wantStart: 95, wantLength: 5,
		},
		{name: "start_past_the_file", header: "bytes=100-", wantOK: true, wantSatisfiable: false},
		{name: "reversed", header: "bytes=20-10", wantOK: false},
		{name: "garbage", header: "bytes=abc", wantOK: false},
		{name: "no_dash", header: "bytes=10", wantOK: false},
		// Multi-range would owe the client a multipart/byteranges body; a
		// resumed download never asks for one, so it falls back to the whole
		// file rather than being answered wrongly.
		{name: "multi_range_falls_back", header: "bytes=0-9,20-29", wantOK: false},
		{name: "whitespace_is_tolerated", header: " bytes= 5 - 9 ", wantOK: true, wantSatisfiable: true, wantStart: 5, wantLength: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rng, ok, satisfiable := parseByteRange(tt.header, size)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantSatisfiable, satisfiable)

			if tt.wantSatisfiable {
				assert.Equal(t, tt.wantStart, rng.start)
				assert.Equal(t, tt.wantLength, rng.length)
			}
		})
	}
}

func TestParseByteRange_an_empty_file_satisfies_nothing(t *testing.T) {
	t.Parallel()

	_, ok, satisfiable := parseByteRange("bytes=-10", 0)
	assert.True(t, ok)
	assert.False(t, satisfiable)
}

func TestByteRange_contentRange(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "bytes 90-99/100", byteRange{start: 90, length: 10}.contentRange(100))
	assert.Equal(t, "bytes 0-0/1", byteRange{start: 0, length: 1}.contentRange(1))
}
