package filters

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPagination(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		limit  uint64
		offset uint64
		want   *Pagination
	}{
		{
			name:   "limit_and_offset",
			limit:  10,
			offset: 30,
			want:   &Pagination{Limit: 10, Offset: 30},
		},
		{
			name:   "zero_values",
			limit:  0,
			offset: 0,
			want:   &Pagination{Limit: 0, Offset: 0},
		},
		{
			name:   "default_values",
			limit:  DefaultLimit,
			offset: DefaultOffset,
			want:   &Pagination{Limit: DefaultLimit, Offset: DefaultOffset},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE done in the table above

			// ACT
			got := NewPagination(tt.limit, tt.offset)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultPagination(t *testing.T) {
	t.Parallel()
	// ASSERT
	assert.Equal(t, DefaultLimit, DefaultPagination.Limit)
	assert.Equal(t, DefaultOffset, DefaultPagination.Offset)
}
