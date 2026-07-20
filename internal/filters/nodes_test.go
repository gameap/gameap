package filters

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindNodeByIDs(t *testing.T) {
	tests := []struct {
		name string
		ids  []uint
		want *FindNode
	}{
		{
			name: "with_ids",
			ids:  []uint{1, 2, 3},
			want: &FindNode{IDs: []uint{1, 2, 3}},
		},
		{
			name: "single_id",
			ids:  []uint{10},
			want: &FindNode{IDs: []uint{10}},
		},
		{
			name: "no_ids",
			ids:  nil,
			want: &FindNode{IDs: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE done in the table above

			// ACT
			got := FindNodeByIDs(tt.ids...)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindNodeByGDaemonAPIKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{name: "with_key", key: "gdaemon-secret-key"},
		{name: "empty_key", key: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE done in the table above

			// ACT
			got := FindNodeByGDaemonAPIKey(tt.key)

			// ASSERT
			require.NotNil(t, got)
			require.NotNil(t, got.GDaemonAPIKey)
			assert.Equal(t, tt.key, *got.GDaemonAPIKey)
			assert.Nil(t, got.IDs)
			assert.Nil(t, got.GDaemonAPIToken)
			assert.False(t, got.WithDeleted)
		})
	}
}
