package filters

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindGameMod_FilterCount(t *testing.T) {
	filterType := reflect.TypeFor[FindGameMod]()
	fieldsCount := filterType.NumField()

	fieldsSet := 0

	newFilterValue := reflect.New(filterType)

	for i := range fieldsCount {
		field := filterType.Field(i)

		if field.Type.Kind() == reflect.Slice {
			sliceValue := reflect.MakeSlice(field.Type, 2, 2)

			fieldValue := newFilterValue.Elem().Field(i)
			fieldValue.Set(sliceValue)

			fieldsSet++
		} else {
			t.Fatal("FindGameMod contains non-slice fields, test needs to be updated")
		}
	}

	filter := newFilterValue.Interface().(*FindGameMod)

	assert.Equal(t, fieldsSet, filter.FilterCount(), "FilterCount should match the number of fields set")
}

func TestFindGameModByGameCodes(t *testing.T) {
	tests := []struct {
		name  string
		codes []string
		want  *FindGameMod
	}{
		{
			name:  "with_codes",
			codes: []string{"cs", "tf2"},
			want:  &FindGameMod{GameCodes: []string{"cs", "tf2"}},
		},
		{
			name:  "single_code",
			codes: []string{"valve"},
			want:  &FindGameMod{GameCodes: []string{"valve"}},
		},
		{
			name:  "no_codes",
			codes: nil,
			want:  &FindGameMod{GameCodes: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE done in the table above

			// ACT
			got := FindGameModByGameCodes(tt.codes...)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}
