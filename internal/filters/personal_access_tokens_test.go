package filters

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindPersonalAccessToken_FilterCount(t *testing.T) {
	t.Parallel()
	filterType := reflect.TypeFor[FindPersonalAccessToken]()
	fieldsCount := filterType.NumField()

	fieldsSet := 0

	newFilterValue := reflect.New(filterType)

	for i := range fieldsCount {
		field := filterType.Field(i)
		t.Logf("Field %d: %s (type: %s)", i, field.Name, field.Type)

		if field.Type.Kind() == reflect.Slice {
			sliceValue := reflect.MakeSlice(field.Type, 2, 2)

			fieldValue := newFilterValue.Elem().Field(i)
			fieldValue.Set(sliceValue)

			fieldsSet++
		} else {
			t.Fatal("FindPersonalAccessToken contains non-slice fields, test needs to be updated")
		}
	}

	filter := newFilterValue.Interface().(*FindPersonalAccessToken)

	assert.Equal(t, fieldsSet, filter.FilterCount(), "FilterCount should match the number of fields set")
}

func TestFindPersonalAccessTokenByIDs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ids  []uint
		want *FindPersonalAccessToken
	}{
		{
			name: "with_ids",
			ids:  []uint{1, 2, 3},
			want: &FindPersonalAccessToken{IDs: []uint{1, 2, 3}},
		},
		{
			name: "single_id",
			ids:  []uint{11},
			want: &FindPersonalAccessToken{IDs: []uint{11}},
		},
		{
			name: "no_ids",
			ids:  nil,
			want: &FindPersonalAccessToken{IDs: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE done in the table above

			// ACT
			got := FindPersonalAccessTokenByIDs(tt.ids...)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}
