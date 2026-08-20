package filters

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindGame_FilterCount(t *testing.T) {
	t.Parallel()
	filterType := reflect.TypeFor[FindGame]()
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
			t.Fatal("FindGame contains non-slice fields, test needs to be updated")
		}
	}

	filter := newFilterValue.Interface().(*FindGame)

	assert.Equal(t, fieldsSet, filter.FilterCount(), "FilterCount should match the number of fields set")
}

func TestFindGameByCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		codes []string
		want  *FindGame
	}{
		{
			name:  "with_codes",
			codes: []string{"cs", "tf2"},
			want:  &FindGame{Codes: []string{"cs", "tf2"}},
		},
		{
			name:  "single_code",
			codes: []string{"minecraft"},
			want:  &FindGame{Codes: []string{"minecraft"}},
		},
		{
			name:  "no_codes",
			codes: nil,
			want:  &FindGame{Codes: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE done in the table above

			// ACT
			got := FindGameByCodes(tt.codes...)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}
