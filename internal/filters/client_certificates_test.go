package filters

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindClientCertificateByIDs(t *testing.T) {
	tests := []struct {
		name string
		ids  []uint
		want *FindClientCertificate
	}{
		{
			name: "with_ids",
			ids:  []uint{1, 2, 3},
			want: &FindClientCertificate{IDs: []uint{1, 2, 3}},
		},
		{
			name: "single_id",
			ids:  []uint{42},
			want: &FindClientCertificate{IDs: []uint{42}},
		},
		{
			name: "no_ids",
			ids:  nil,
			want: &FindClientCertificate{IDs: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE done in the table above

			// ACT
			got := FindClientCertificateByIDs(tt.ids...)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}
