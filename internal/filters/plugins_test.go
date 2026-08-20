package filters

import (
	"reflect"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestFindPlugin_FilterCount(t *testing.T) {
	t.Parallel()
	filterType := reflect.TypeFor[FindPlugin]()
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
			t.Fatal("FindPlugin contains non-slice fields, test needs to be updated")
		}
	}

	filter := newFilterValue.Interface().(*FindPlugin)

	assert.Equal(t, fieldsSet, filter.FilterCount(), "FilterCount should match the number of fields set")
}

func TestFindPluginByIDs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ids  []domain.Uint64ID
		want *FindPlugin
	}{
		{
			name: "with_ids",
			ids:  []domain.Uint64ID{1, 2, 3},
			want: &FindPlugin{IDs: []domain.Uint64ID{1, 2, 3}},
		},
		{
			name: "single_id",
			ids:  []domain.Uint64ID{9},
			want: &FindPlugin{IDs: []domain.Uint64ID{9}},
		},
		{
			name: "no_ids",
			ids:  nil,
			want: &FindPlugin{IDs: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE done in the table above

			// ACT
			got := FindPluginByIDs(tt.ids...)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindPluginByNames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		names []string
		want  *FindPlugin
	}{
		{
			name:  "with_names",
			names: []string{"plugin1", "plugin2"},
			want:  &FindPlugin{Names: []string{"plugin1", "plugin2"}},
		},
		{
			name:  "single_name",
			names: []string{"awesome-plugin"},
			want:  &FindPlugin{Names: []string{"awesome-plugin"}},
		},
		{
			name:  "no_names",
			names: nil,
			want:  &FindPlugin{Names: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE done in the table above

			// ACT
			got := FindPluginByNames(tt.names...)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindPluginByStatuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		statuses []domain.PluginStatus
		want     *FindPlugin
	}{
		{
			name:     "with_statuses",
			statuses: []domain.PluginStatus{domain.PluginStatusActive, domain.PluginStatusDisabled},
			want:     &FindPlugin{Statuses: []domain.PluginStatus{domain.PluginStatusActive, domain.PluginStatusDisabled}},
		},
		{
			name:     "single_status",
			statuses: []domain.PluginStatus{domain.PluginStatusError},
			want:     &FindPlugin{Statuses: []domain.PluginStatus{domain.PluginStatusError}},
		},
		{
			name:     "no_statuses",
			statuses: nil,
			want:     &FindPlugin{Statuses: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE done in the table above

			// ACT
			got := FindPluginByStatuses(tt.statuses...)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}
