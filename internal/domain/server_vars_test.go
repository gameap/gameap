package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerVars_Scan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     any
		want      ServerVars
		wantError string
	}{
		{
			name:  "nil_value",
			value: nil,
			want:  nil,
		},
		{
			name:  "empty_bytes",
			value: []byte{},
			want:  nil,
		},
		{
			name:  "empty_string",
			value: "",
			want:  nil,
		},
		{
			name:  "valid_json_bytes",
			value: []byte(`{"maxplayers":"32","hostname":"Test Server"}`),
			want:  ServerVars{"maxplayers": "32", "hostname": "Test Server"},
		},
		{
			name:  "valid_json_string",
			value: `{"key":"value"}`,
			want:  ServerVars{"key": "value"},
		},
		{
			name:      "invalid_json",
			value:     []byte(`{invalid}`),
			wantError: "invalid character",
		},
		{
			name:  "unsupported_type",
			value: 123,
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var vars ServerVars
			err := vars.Scan(tt.value)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, vars)
		})
	}
}

func TestServerVars_Value(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		vars ServerVars
		want string
	}{
		{
			name: "nil_map",
			vars: nil,
			want: "",
		},
		{
			name: "empty_map",
			vars: ServerVars{},
			want: "{}",
		},
		{
			name: "single_key",
			vars: ServerVars{"key": "value"},
			want: `{"key":"value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := tt.vars.Value()
			require.NoError(t, err)

			if tt.want == "" {
				assert.Nil(t, result)
			} else {
				bytes, ok := result.([]byte)
				require.True(t, ok)
				assert.JSONEq(t, tt.want, string(bytes))
			}
		})
	}
}

func TestServerVars_StringPtr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		vars ServerVars
		want *string
	}{
		{
			name: "nil_map",
			vars: nil,
			want: nil,
		},
		{
			name: "valid_map",
			vars: ServerVars{"maxplayers": "32"},
			want: new(`{"maxplayers":"32"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.vars.StringPtr()

			if tt.want == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.JSONEq(t, *tt.want, *result)
			}
		})
	}
}
