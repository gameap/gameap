package api //nolint:revive,nolintlint

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInputReader_ReadUint(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

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
	t.Parallel()

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
			t.Parallel()

			reader := &InputReader{vars: tt.vars}

			result, err := reader.ReadString(tt.key)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInputReader_ReadList(t *testing.T) {
	t.Parallel()

	reader := &InputReader{vars: map[string]string{"key": "value"}}

	result, err := reader.ReadList("key")

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestNewInputReader(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/test/123", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "123"})

	reader := NewInputReader(req)

	require.NotNil(t, reader)
	result, err := reader.ReadUint("id")
	require.NoError(t, err)
	assert.Equal(t, uint(123), result)
}

func TestNewQueryReader(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/test?name=value", nil)

	reader := NewQueryReader(req)

	require.NotNil(t, reader)
	result, err := reader.ReadString("name")
	require.NoError(t, err)
	assert.Equal(t, "value", result)
}

func TestQueryReader_ReadString(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			reader := &QueryReader{query: tt.query}

			result, err := reader.ReadString(tt.key)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestQueryReader_ReadList(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			reader := &QueryReader{query: tt.query}

			result, err := reader.ReadList(tt.key)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestQueryReader_ReadIntList(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

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
	t.Parallel()

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
			t.Parallel()

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

func newMultipartRequest(t *testing.T, fields map[string]string) *http.Request {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", "plugin.wasm")
	require.NoError(t, err)
	_, err = part.Write([]byte{0x00, 0x61, 0x73, 0x6d})
	require.NoError(t, err)

	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(1<<20))

	return req
}

func TestFormReader_ReadBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fields    map[string]string
		key       string
		expected  bool
		wantError string
	}{
		{
			name:     "missing_key_is_false",
			fields:   nil,
			key:      "update",
			expected: false,
		},
		{
			name:     "empty_value_is_false",
			fields:   map[string]string{"update": ""},
			key:      "update",
			expected: false,
		},
		{
			name:     "true_literal",
			fields:   map[string]string{"update": "true"},
			key:      "update",
			expected: true,
		},
		{
			name:     "numeric_one_is_true",
			fields:   map[string]string{"update": "1"},
			key:      "update",
			expected: true,
		},
		{
			name:     "false_literal",
			fields:   map[string]string{"update": "false"},
			key:      "update",
			expected: false,
		},
		{
			name:      "unparsable_value_is_rejected",
			fields:    map[string]string{"update": "maybe"},
			key:       "update",
			wantError: "value is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reader := NewFormReader(newMultipartRequest(t, tt.fields))

			result, err := reader.ReadBool(tt.key)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormReader_ReadString(t *testing.T) {
	t.Parallel()

	reader := NewFormReader(newMultipartRequest(t, map[string]string{"config": `{"a":1}`}))

	value, err := reader.ReadString("config")
	require.NoError(t, err)
	assert.Equal(t, `{"a":1}`, value)

	missing, err := reader.ReadString("absent")
	require.NoError(t, err)
	assert.Empty(t, missing)
}

// TestFormReader_unparsed_request reads nothing rather than panicking: the
// caller is expected to parse the form first, and a handler that forgets must
// not take the process down.
func TestFormReader_unparsed_request(t *testing.T) {
	t.Parallel()

	reader := NewFormReader(httptest.NewRequest(http.MethodPost, "/upload", nil))

	value, err := reader.ReadBool("update")
	require.NoError(t, err)
	assert.False(t, value)
}
