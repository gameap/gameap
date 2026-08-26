// Unit tests for the user-input sort parser. Covers OWASP API8:2023
// (Security Misconfiguration) and CWE-89 (SQL Injection via ORDER BY): the
// allow-list at this layer is the project's central defence against an attacker
// smuggling SQL fragments through a `sort` query parameter.
package filters_test

import (
	"testing"

	"github.com/gameap/gameap/internal/filters"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseUserSort(t *testing.T) {
	t.Parallel()
	allowed := map[string]string{
		"id":        "id",
		"name":      "name",
		"server_ip": "server_ip",
	}

	tests := []struct {
		name        string
		raw         string
		wantField   string
		wantDir     filters.SortDirection
		wantNil     bool
		wantErrorIs error
	}{
		{name: "empty_returns_nil_no_error", raw: "", wantNil: true},
		{name: "whitespace_only_returns_nil_no_error", raw: "  ", wantNil: true},
		{name: "asc_simple", raw: "id", wantField: "id", wantDir: filters.SortDirectionAsc},
		{name: "desc_with_minus_prefix", raw: "-name", wantField: "name", wantDir: filters.SortDirectionDesc},
		{name: "alias_preserved", raw: "server_ip", wantField: "server_ip", wantDir: filters.SortDirectionAsc},
		{name: "rejects_unknown_field", raw: "secret_column", wantErrorIs: filters.ErrInvalidSortField},
		{name: "rejects_sql_injection_payload", raw: "id;DROP TABLE users--", wantErrorIs: filters.ErrInvalidSortField},
		{name: "rejects_sql_injection_with_union", raw: "id UNION SELECT 1", wantErrorIs: filters.ErrInvalidSortField},
		{name: "rejects_path_traversal_payload", raw: "../../etc/passwd", wantErrorIs: filters.ErrInvalidSortField},
		{name: "rejects_field_with_space", raw: "id name", wantErrorIs: filters.ErrInvalidSortField},
		{name: "case_sensitive_rejects_uppercase", raw: "ID", wantErrorIs: filters.ErrInvalidSortField},
		{name: "minus_only_rejected", raw: "-", wantErrorIs: filters.ErrInvalidSortField},
		{name: "double_minus_rejected", raw: "--id", wantErrorIs: filters.ErrInvalidSortField},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := filters.ParseUserSort(tt.raw, allowed)

			if tt.wantErrorIs != nil {
				require.Error(t, err)
				assert.Truef(t, errors.Is(err, tt.wantErrorIs),
					"expected %v in chain, got %v", tt.wantErrorIs, err)
				assert.Nil(t, got)

				return
			}

			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, got)

				return
			}

			require.NotNil(t, got)
			assert.Equal(t, tt.wantField, got.Field)
			assert.Equal(t, tt.wantDir, got.Direction)
		})
	}
}

func TestParseUserSort_AllowedAliasMapping(t *testing.T) {
	t.Parallel()
	// Map the public field "createdAt" to the physical column "created_at".
	got, err := filters.ParseUserSort("-createdAt", map[string]string{"createdAt": "created_at"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "created_at", got.Field)
	assert.Equal(t, filters.SortDirectionDesc, got.Direction)
}

func TestParseUserSort_EmptyMappedColumnRejected(t *testing.T) {
	t.Parallel()
	// A misconfigured allow-list with an empty target column must not be honoured.
	got, err := filters.ParseUserSort("id", map[string]string{"id": ""})
	require.Error(t, err)
	assert.Truef(t, errors.Is(err, filters.ErrInvalidSortField),
		"expected ErrInvalidSortField, got %v", err)
	assert.Nil(t, got)
}

func TestSortDirection_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		direction filters.SortDirection
		want      string
	}{
		{name: "asc", direction: filters.SortDirectionAsc, want: "asc"},
		{name: "desc", direction: filters.SortDirectionDesc, want: "desc"},
		{name: "unknown_returns_empty", direction: filters.SortDirection(42), want: ""},
		{name: "negative_returns_empty", direction: filters.SortDirection(-1), want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE done in the table above

			// ACT
			got := tt.direction.String()

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewSorting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		field     string
		direction filters.SortDirection
		want      *filters.Sorting
	}{
		{
			name:      "asc_field",
			field:     "name",
			direction: filters.SortDirectionAsc,
			want:      &filters.Sorting{Field: "name", Direction: filters.SortDirectionAsc},
		},
		{
			name:      "desc_field",
			field:     "created_at",
			direction: filters.SortDirectionDesc,
			want:      &filters.Sorting{Field: "created_at", Direction: filters.SortDirectionDesc},
		},
		{
			name:      "empty_field",
			field:     "",
			direction: filters.SortDirectionAsc,
			want:      &filters.Sorting{Field: "", Direction: filters.SortDirectionAsc},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE done in the table above

			// ACT
			got := filters.NewSorting(tt.field, tt.direction)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSorting_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		sorting *filters.Sorting
		want    string
	}{
		{name: "asc", sorting: filters.NewSorting("name", filters.SortDirectionAsc), want: "name asc"},
		{name: "desc", sorting: filters.NewSorting("id", filters.SortDirectionDesc), want: "id desc"},
		{name: "empty_field", sorting: filters.NewSorting("", filters.SortDirectionAsc), want: " asc"},
		{name: "unknown_direction", sorting: filters.NewSorting("name", filters.SortDirection(7)), want: "name "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE done in the table above

			// ACT
			got := tt.sorting.String()

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSorting_String_ParseUserSortCompatible(t *testing.T) {
	t.Parallel()
	// ARRANGE
	allowed := map[string]string{"createdAt": "created_at"}

	// ACT
	sorting, err := filters.ParseUserSort("-createdAt", allowed)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, sorting)
	assert.Equal(t, "created_at desc", sorting.String())
}
