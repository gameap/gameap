package idempotency_test

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/gameap/gameap/internal/idempotency"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiddleware_distinguishes_typed_JSON_inputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		firstBody string
		retryBody string
	}{
		{
			name:      "case_insensitive_struct_fields",
			firstBody: `{"name":"First","Name":"Second"}`,
			retryBody: `{"Name":"Second","name":"First"}`,
		},
		{
			name:      "duplicate_map_fields_merge_in_structs",
			firstBody: `{"vars":{"a":"1"},"vars":{"b":"2"}}`,
			retryBody: `{"vars":{"b":"2"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var firstInput, retryInput struct {
				Name string            `json:"name"`
				Vars map[string]string `json:"vars"`
			}
			require.NoError(t, json.Unmarshal([]byte(tt.firstBody), &firstInput))
			require.NoError(t, json.Unmarshal([]byte(tt.retryBody), &retryInput))
			require.NotEqual(t, firstInput, retryInput)

			var calls atomic.Int32
			h := newTestMiddleware(t, nil, nil).Middleware(createHandler(&calls))
			first := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", tt.firstBody, 1))
			require.Equal(t, http.StatusCreated, first.Code)

			retry := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", tt.retryBody, 1))
			assert.Equal(t, http.StatusUnprocessableEntity, retry.Code)
			assert.Empty(t, retry.Header().Get(idempotency.HeaderReplayed))
			assert.Equal(t, int32(1), calls.Load())
		})
	}
}
