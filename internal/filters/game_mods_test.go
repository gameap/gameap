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
