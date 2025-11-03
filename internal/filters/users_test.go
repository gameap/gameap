package filters

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindUser_FilterCount(t *testing.T) {
	filterType := reflect.TypeFor[FindUser]()
	fieldsCount := filterType.NumField()

	fieldsSet := 0

	newFilterValue := reflect.New(filterType)

	for i := range fieldsCount {
		field := filterType.Field(i)
		t.Logf("Field %d: %s (type: %s)", i, field.Name, field.Type)

		if field.Type.Kind() == reflect.Slice {
			elemType := field.Type.Elem()

			var sliceValue reflect.Value
			switch elemType.Kind() {
			case reflect.Uint:
				sliceValue = reflect.ValueOf([]uint{1, 2})
			case reflect.String:
				sliceValue = reflect.ValueOf([]string{"default1", "default2"})
			default:
				sliceValue = reflect.MakeSlice(field.Type, 2, 2)
			}

			fieldValue := newFilterValue.Elem().Field(i)
			fieldValue.Set(sliceValue)

			fieldsSet++
		} else {
			t.Fatal("FindUser contains non-slice fields, test needs to be updated")
		}
	}

	filter := newFilterValue.Interface().(*FindUser)

	assert.Equal(t, fieldsSet, filter.FilterCount(), "FilterCount should match the number of fields set")
}
