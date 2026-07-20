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

func TestFindUserByIDs(t *testing.T) {
	tests := []struct {
		name string
		ids  []uint
		want *FindUser
	}{
		{
			name: "with_ids",
			ids:  []uint{1, 2, 3},
			want: &FindUser{IDs: []uint{1, 2, 3}},
		},
		{
			name: "single_id",
			ids:  []uint{42},
			want: &FindUser{IDs: []uint{42}},
		},
		{
			name: "no_ids",
			ids:  nil,
			want: &FindUser{IDs: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE done in the table above

			// ACT
			got := FindUserByIDs(tt.ids...)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindUserByLogins(t *testing.T) {
	tests := []struct {
		name   string
		logins []string
		want   *FindUser
	}{
		{
			name:   "with_logins",
			logins: []string{"admin", "user1"},
			want:   &FindUser{Logins: []string{"admin", "user1"}},
		},
		{
			name:   "single_login",
			logins: []string{"root"},
			want:   &FindUser{Logins: []string{"root"}},
		},
		{
			name:   "no_logins",
			logins: nil,
			want:   &FindUser{Logins: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE done in the table above

			// ACT
			got := FindUserByLogins(tt.logins...)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindUserByEmails(t *testing.T) {
	tests := []struct {
		name   string
		emails []string
		want   *FindUser
	}{
		{
			name:   "with_emails",
			emails: []string{"admin@example.com", "user@example.com"},
			want:   &FindUser{Emails: []string{"admin@example.com", "user@example.com"}},
		},
		{
			name:   "single_email",
			emails: []string{"test@example.com"},
			want:   &FindUser{Emails: []string{"test@example.com"}},
		},
		{
			name:   "no_emails",
			emails: nil,
			want:   &FindUser{Emails: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE done in the table above

			// ACT
			got := FindUserByEmails(tt.emails...)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}
