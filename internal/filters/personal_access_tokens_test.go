package filters

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindPersonalAccessToken_FilterCount(t *testing.T) {
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
