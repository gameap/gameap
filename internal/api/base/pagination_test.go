package base_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadPage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		wantNumber int
		wantSize   int
		wantError  string
	}{
		{
			name:       "defaults_when_absent",
			query:      "",
			wantNumber: 1,
			wantSize:   base.DefaultPageSize,
		},
		{
			name:       "explicit_values_within_bounds",
			query:      "page[number]=3&page[size]=50",
			wantNumber: 3,
			wantSize:   50,
		},
		{
			name:       "oversized_page_size_is_clamped",
			query:      "page[size]=1000000000",
			wantNumber: 1,
			wantSize:   base.MaxPageSize,
		},
		{
			name:       "page_size_at_the_cap_is_kept",
			query:      "page[size]=100",
			wantNumber: 1,
			wantSize:   base.MaxPageSize,
		},
		{
			name:      "non_positive_page_size_errors",
			query:     "page[size]=0",
			wantError: "page[size] must be positive",
		},
		{
			name:      "non_positive_page_number_errors",
			query:     "page[number]=0",
			wantError: "page[number] must be positive",
		},
		{
			name:      "malformed_page_size_errors",
			query:     "page[size]=abc",
			wantError: "invalid page[size] value",
		},
		{
			name:      "malformed_page_number_errors",
			query:     "page[number]=abc",
			wantError: "invalid page[number] value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/api/servers?"+tt.query, http.NoBody)
			number, size, err := base.ReadPage(api.NewQueryReader(req))

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantNumber, number)
			assert.Equal(t, tt.wantSize, size)
		})
	}
}
