package filters

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestFindServerByIDs(t *testing.T) {
	tests := []struct {
		name string
		ids  []uint
		want *FindServer
	}{
		{
			name: "with_ids",
			ids:  []uint{1, 2, 3},
			want: &FindServer{IDs: []uint{1, 2, 3}},
		},
		{
			name: "single_id",
			ids:  []uint{5},
			want: &FindServer{IDs: []uint{5}},
		},
		{
			name: "no_ids",
			ids:  nil,
			want: &FindServer{IDs: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE done in the table above

			// ACT
			got := FindServerByIDs(tt.ids...)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindServerByNodeIDs(t *testing.T) {
	tests := []struct {
		name    string
		nodeIDs []uint
		want    *FindServer
	}{
		{
			name:    "with_node_ids",
			nodeIDs: []uint{1, 2},
			want:    &FindServer{DSIDs: []uint{1, 2}},
		},
		{
			name:    "single_node_id",
			nodeIDs: []uint{3},
			want:    &FindServer{DSIDs: []uint{3}},
		},
		{
			name:    "no_node_ids",
			nodeIDs: nil,
			want:    &FindServer{DSIDs: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE done in the table above

			// ACT
			got := FindServerByNodeIDs(tt.nodeIDs...)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindServerByUUIDs(t *testing.T) {
	uuid1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	uuid2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	tests := []struct {
		name  string
		uuids []uuid.UUID
		want  *FindServer
	}{
		{
			name:  "with_uuids",
			uuids: []uuid.UUID{uuid1, uuid2},
			want:  &FindServer{UUIDs: []uuid.UUID{uuid1, uuid2}},
		},
		{
			name:  "single_uuid",
			uuids: []uuid.UUID{uuid1},
			want:  &FindServer{UUIDs: []uuid.UUID{uuid1}},
		},
		{
			name:  "empty_uuids",
			uuids: []uuid.UUID{},
			want:  &FindServer{UUIDs: []uuid.UUID{}},
		},
		{
			name:  "nil_uuids",
			uuids: nil,
			want:  &FindServer{UUIDs: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE done in the table above

			// ACT
			got := FindServerByUUIDs(tt.uuids)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}
