package domain_test

import (
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNode_PortRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		node      domain.Node
		want      domain.PortRange
		wantError string
	}{
		{
			name: "nil_metadata",
			node: domain.Node{},
			want: nil,
		},
		{
			name: "other_keys_only",
			node: domain.Node{Metadata: domain.Metadata{"hetzner.server_id": "123"}},
			want: nil,
		},
		{
			name: "null_value",
			node: domain.Node{Metadata: domain.Metadata{"port_range": nil}},
			want: nil,
		},
		{
			name: "empty_string",
			node: domain.Node{Metadata: domain.Metadata{"port_range": ""}},
			want: nil,
		},
		{
			name: "valid_pool",
			node: domain.Node{Metadata: domain.Metadata{"port_range": "27015-27020, 28000"}},
			want: domain.PortRange{{From: 27015, To: 27020}, {From: 28000, To: 28000}},
		},
		{
			name:      "number_instead_of_string",
			node:      domain.Node{Metadata: domain.Metadata{"port_range": float64(27015)}},
			wantError: "port range must list ports and ranges within 1-65535",
		},
		{
			name:      "unparsable_string",
			node:      domain.Node{Metadata: domain.Metadata{"port_range": "27015-"}},
			wantError: `invalid entry "27015-"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.node.PortRange()

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
