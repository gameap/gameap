package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInputReader_ReadUint(t *testing.T) {
	tests := []struct {
		name        string
		vars        map[string]string
		key         string
		expected    uint
		expectError bool
	}{
		{
			name:     "valid_uint",
			vars:     map[string]string{"id": "123"},
			key:      "id",
			expected: 123,
		},
		{
			name:     "zero_value",
			vars:     map[string]string{"id": "0"},
			key:      "id",
			expected: 0,
		},
		{
			name:        "negative_value",
			vars:        map[string]string{"id": "-1"},
			key:         "id",
			expectError: true,
		},
		{
			name:        "non_numeric_value",
			vars:        map[string]string{"id": "abc"},
			key:         "id",
			expectError: true,
		},
		{
			name:        "empty_value",
			vars:        map[string]string{"id": ""},
			key:         "id",
			expectError: true,
		},
		{
			name:        "missing_key",
			vars:        map[string]string{},
			key:         "id",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &InputReader{vars: tt.vars}

			result, err := reader.ReadUint(tt.key)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestInputReader_ReadString(t *testing.T) {
	tests := []struct {
		name     string
		vars     map[string]string
		key      string
		expected string
	}{
		{
			name:     "existing_key",
			vars:     map[string]string{"name": "test"},
			key:      "name",
			expected: "test",
		},
		{
			name:     "missing_key",
			vars:     map[string]string{},
			key:      "name",
			expected: "",
		},
		{
			name:     "empty_value",
			vars:     map[string]string{"name": ""},
			key:      "name",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &InputReader{vars: tt.vars}

			result, err := reader.ReadString(tt.key)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInputReader_ReadList(t *testing.T) {
	reader := &InputReader{vars: map[string]string{"key": "value"}}

	result, err := reader.ReadList("key")

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestNewInputReader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test/123", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "123"})

	reader := NewInputReader(req)

	require.NotNil(t, reader)
	result, err := reader.ReadUint("id")
	require.NoError(t, err)
	assert.Equal(t, uint(123), result)
}

func TestNewQueryReader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?name=value", nil)

	reader := NewQueryReader(req)

	require.NotNil(t, reader)
	result, err := reader.ReadString("name")
	require.NoError(t, err)
	assert.Equal(t, "value", result)
}

func TestQueryReader_ReadString(t *testing.T) {
	tests := []struct {
		name     string
		query    map[string][]string
		key      string
		expected string
	}{
		{
			name:     "existing_key",
			query:    map[string][]string{"name": {"test"}},
			key:      "name",
			expected: "test",
		},
		{
			name:     "missing_key",
			query:    map[string][]string{},
			key:      "name",
			expected: "",
		},
		{
			name:     "empty_slice",
			query:    map[string][]string{"name": {}},
			key:      "name",
			expected: "",
		},
		{
			name:     "multiple_values_returns_first",
			query:    map[string][]string{"name": {"first", "second"}},
			key:      "name",
			expected: "first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &QueryReader{query: tt.query}

			result, err := reader.ReadString(tt.key)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestQueryReader_ReadList(t *testing.T) {
	tests := []struct {
		name     string
		query    map[string][]string
		key      string
		expected []string
	}{
		{
			name:     "single_value",
			query:    map[string][]string{"ids": {"1"}},
			key:      "ids",
			expected: []string{"1"},
		},
		{
			name:     "multiple_values",
			query:    map[string][]string{"ids": {"1", "2", "3"}},
			key:      "ids",
			expected: []string{"1", "2", "3"},
		},
		{
			name:     "comma_separated_values",
			query:    map[string][]string{"ids": {"1,2,3"}},
			key:      "ids",
			expected: []string{"1", "2", "3"},
		},
		{
			name:     "mixed_comma_and_separate_values",
			query:    map[string][]string{"ids": {"1,2", "3"}},
			key:      "ids",
			expected: []string{"1", "2", "3"},
		},
		{
			name:     "bracket_notation",
			query:    map[string][]string{"ids[]": {"1", "2"}},
			key:      "ids",
			expected: []string{"1", "2"},
		},
		{
			name:     "bracket_notation_with_comma",
			query:    map[string][]string{"ids[]": {"1,2"}},
			key:      "ids",
			expected: []string{"1", "2"},
		},
		{
			name:     "missing_key",
			query:    map[string][]string{},
			key:      "ids",
			expected: []string{},
		},
		{
			name:     "empty_slice",
			query:    map[string][]string{"ids": {}},
			key:      "ids",
			expected: []string{},
		},
		{
			name:     "prefers_non_bracket_key",
			query:    map[string][]string{"ids": {"1"}, "ids[]": {"2"}},
			key:      "ids",
			expected: []string{"1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &QueryReader{query: tt.query}

			result, err := reader.ReadList(tt.key)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestQueryReader_ReadIntList(t *testing.T) {
	tests := []struct {
		name        string
		query       map[string][]string
		key         string
		expected    []int
		expectError bool
	}{
		{
			name:     "single_value",
			query:    map[string][]string{"ids": {"1"}},
			key:      "ids",
			expected: []int{1},
		},
		{
			name:     "multiple_values",
			query:    map[string][]string{"ids": {"1", "2", "3"}},
			key:      "ids",
			expected: []int{1, 2, 3},
		},
		{
			name:     "comma_separated_values",
			query:    map[string][]string{"ids": {"1,2,3"}},
			key:      "ids",
			expected: []int{1, 2, 3},
		},
		{
			name:     "negative_values_allowed",
			query:    map[string][]string{"ids": {"-1", "0", "1"}},
			key:      "ids",
			expected: []int{-1, 0, 1},
		},
		{
			name:     "missing_key",
			query:    map[string][]string{},
			key:      "ids",
			expected: []int{},
		},
		{
			name:     "empty_values_skipped",
			query:    map[string][]string{"ids": {"1", "", "2"}},
			key:      "ids",
			expected: []int{1, 2},
		},
		{
			name:        "non_numeric_value",
			query:       map[string][]string{"ids": {"1", "abc", "2"}},
			key:         "ids",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &QueryReader{query: tt.query}

			result, err := reader.ReadIntList(tt.key)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestQueryReader_ReadUintList(t *testing.T) {
	tests := []struct {
		name        string
		query       map[string][]string
		key         string
		expected    []uint
		expectError bool
	}{
		{
			name:     "single_value",
			query:    map[string][]string{"ids": {"1"}},
			key:      "ids",
			expected: []uint{1},
		},
		{
			name:     "multiple_values",
			query:    map[string][]string{"ids": {"1", "2", "3"}},
			key:      "ids",
			expected: []uint{1, 2, 3},
		},
		{
			name:     "comma_separated_values",
			query:    map[string][]string{"ids": {"1,2,3"}},
			key:      "ids",
			expected: []uint{1, 2, 3},
		},
		{
			name:     "zero_value",
			query:    map[string][]string{"ids": {"0"}},
			key:      "ids",
			expected: []uint{0},
		},
		{
			name:     "missing_key",
			query:    map[string][]string{},
			key:      "ids",
			expected: []uint{},
		},
		{
			name:     "empty_values_skipped",
			query:    map[string][]string{"ids": {"1", "", "2"}},
			key:      "ids",
			expected: []uint{1, 2},
		},
		{
			name:        "negative_value",
			query:       map[string][]string{"ids": {"-1"}},
			key:         "ids",
			expectError: true,
		},
		{
			name:        "non_numeric_value",
			query:       map[string][]string{"ids": {"abc"}},
			key:         "ids",
			expectError: true,
		},
		{
			name:     "bracket_notation_with_comma",
			query:    map[string][]string{"ids[]": {"1,2,3"}},
			key:      "ids",
			expected: []uint{1, 2, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &QueryReader{query: tt.query}

			result, err := reader.ReadUintList(tt.key)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
