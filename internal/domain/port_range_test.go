package domain_test

import (
	"slices"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePortRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		want      domain.PortRange
		wantError string
	}{
		{
			name:  "blank_is_empty_pool",
			input: "  ",
			want:  nil,
		},
		{
			name:  "single_port",
			input: "27015",
			want:  domain.PortRange{{From: 27015, To: 27015}},
		},
		{
			name:  "inclusive_range",
			input: "27015-27020",
			want:  domain.PortRange{{From: 27015, To: 27020}},
		},
		{
			name:  "spaces_around_entries_and_dash",
			input: " 27015 - 27020 , 28000 ",
			want:  domain.PortRange{{From: 27015, To: 27020}, {From: 28000, To: 28000}},
		},
		{
			name:  "spans_are_sorted",
			input: "28000, 27015-27020",
			want:  domain.PortRange{{From: 27015, To: 27020}, {From: 28000, To: 28000}},
		},
		{
			name:  "overlapping_spans_are_merged",
			input: "27015-27030, 27020-27040, 27025",
			want:  domain.PortRange{{From: 27015, To: 27040}},
		},
		{
			name:  "adjacent_spans_are_merged",
			input: "27015-27020, 27021-27025",
			want:  domain.PortRange{{From: 27015, To: 27025}},
		},
		{
			name:  "whole_port_space",
			input: "1-65535",
			want:  domain.PortRange{{From: 1, To: 65535}},
		},
		{
			name:      "port_zero",
			input:     "0",
			wantError: `invalid entry "0": port range must list ports and ranges within 1-65535`,
		},
		{
			name:      "port_above_max",
			input:     "27015-65536",
			wantError: `invalid entry "27015-65536"`,
		},
		{
			name:      "reversed_range",
			input:     "27020-27015",
			wantError: `invalid entry "27020-27015"`,
		},
		{
			name:      "empty_entry",
			input:     "27015,,27020",
			wantError: `invalid entry ""`,
		},
		{
			name:      "trailing_comma",
			input:     "27015,",
			wantError: `invalid entry ""`,
		},
		{
			name:      "open_range",
			input:     "27015-",
			wantError: `invalid entry "27015-"`,
		},
		{
			name:      "double_dash",
			input:     "27015--27020",
			wantError: `invalid entry "27015--27020"`,
		},
		{
			name:      "signed_port",
			input:     "+27015",
			wantError: `invalid entry "+27015"`,
		},
		{
			name:      "not_a_number",
			input:     "game",
			wantError: `invalid entry "game"`,
		},
		{
			name:      "semicolon_separator",
			input:     "27015;27020",
			wantError: `invalid entry "27015;27020"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.ParsePortRange(tt.input)

			if tt.wantError != "" {
				require.Error(t, err)
				require.ErrorIs(t, err, domain.ErrPortRangeInvalid)
				assert.Contains(t, err.Error(), tt.wantError)
				assert.Nil(t, got)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPortRange_All(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []int
	}{
		{
			name:  "empty_pool",
			input: "",
			want:  nil,
		},
		{
			name:  "ascending_across_spans",
			input: "28000-28001, 27015-27017",
			want:  []int{27015, 27016, 27017, 28000, 28001},
		},
		{
			name:  "overlap_visits_each_port_once",
			input: "27015-27017, 27016-27018",
			want:  []int{27015, 27016, 27017, 27018},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool, err := domain.ParsePortRange(tt.input)
			require.NoError(t, err)

			assert.Equal(t, tt.want, slices.Collect(pool.All()))
		})
	}
}

func TestPortRange_All_StopsWhenAsked(t *testing.T) {
	t.Parallel()

	pool, err := domain.ParsePortRange("27015-27100")
	require.NoError(t, err)

	var got []int
	for port := range pool.All() {
		got = append(got, port)
		if len(got) == 2 {
			break
		}
	}

	assert.Equal(t, []int{27015, 27016}, got)
}
