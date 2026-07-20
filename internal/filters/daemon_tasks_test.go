package filters

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindDaemonTaskByIDs(t *testing.T) {
	tests := []struct {
		name string
		ids  []uint
		want *FindDaemonTask
	}{
		{
			name: "with_ids",
			ids:  []uint{1, 2, 3},
			want: &FindDaemonTask{IDs: []uint{1, 2, 3}},
		},
		{
			name: "single_id",
			ids:  []uint{7},
			want: &FindDaemonTask{IDs: []uint{7}},
		},
		{
			name: "no_ids",
			ids:  nil,
			want: &FindDaemonTask{IDs: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE done in the table above

			// ACT
			got := FindDaemonTaskByIDs(tt.ids...)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}
