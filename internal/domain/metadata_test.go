package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetadata_Scan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		receiver Metadata
		input    any
		expected Metadata
		wantErr  bool
	}{
		{
			name:     "nil_value",
			receiver: Metadata{"preexisting": "x"},
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty_bytes",
			receiver: Metadata{"preexisting": "x"},
			input:    []byte{},
			expected: nil,
		},
		{
			name:     "empty_string",
			receiver: Metadata{"preexisting": "x"},
			input:    "",
			expected: nil,
		},
		{
			name:     "valid_json_bytes",
			receiver: nil,
			input:    []byte(`{"docker_image":"ghcr.io/gameap/csgo:latest","default_port":27015}`),
			expected: Metadata{"docker_image": "ghcr.io/gameap/csgo:latest", "default_port": float64(27015)},
		},
		{
			name:     "valid_json_string",
			receiver: nil,
			input:    `{"key":"value"}`,
			expected: Metadata{"key": "value"},
		},
		{
			name:     "nested_object",
			receiver: nil,
			input:    []byte(`{"config":{"enabled":true,"ports":[27015,27016]}}`),
			expected: Metadata{"config": map[string]any{"enabled": true, "ports": []any{float64(27015), float64(27016)}}},
		},
		{
			name:     "unsupported_type_leaves_receiver_unchanged",
			receiver: Metadata{"preexisting": "x"},
			input:    123,
			expected: Metadata{"preexisting": "x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			m := tt.receiver

			// ACT
			err := m.Scan(tt.input)

			// ASSERT
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, m, "metadata receiver state mismatch")
		})
	}
}

func TestMetadata_Value(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		metadata  Metadata
		expected  string
		expectNil bool
	}{
		{
			name:      "nil_metadata",
			metadata:  nil,
			expectNil: true,
		},
		{
			name:     "simple_object",
			metadata: Metadata{"key": "value"},
			expected: `{"key":"value"}`,
		},
		{
			name:     "complex_object",
			metadata: Metadata{"docker_image": "test:latest", "port": float64(27015)},
			expected: `{"docker_image":"test:latest","port":27015}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			val, err := tt.metadata.Value()
			require.NoError(t, err)

			if tt.expectNil {
				assert.Nil(t, val)

				return
			}

			bytes, ok := val.([]byte)
			require.True(t, ok)
			assert.JSONEq(t, tt.expected, string(bytes))
		})
	}
}

func TestMetadata_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata Metadata
		expected string
	}{
		{
			name:     "nil_metadata",
			metadata: nil,
			expected: "",
		},
		{
			name:     "empty_metadata",
			metadata: Metadata{},
			expected: "{}",
		},
		{
			name:     "simple_object",
			metadata: Metadata{"key": "value"},
			expected: `{"key":"value"}`,
		},
		{
			name:     "metadata_with_channel_returns_empty",
			metadata: Metadata{"key": make(chan int)},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE / ACT
			result := tt.metadata.String()

			// ASSERT
			if tt.expected == "" {
				assert.Empty(t, result, "expected empty string for un-marshalable / nil metadata")

				return
			}

			assert.JSONEq(t, tt.expected, result)
		})
	}
}
