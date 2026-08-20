package ws

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOutboundMessage(t *testing.T) {
	t.Parallel()
	before := time.Now().Unix()
	msg := NewOutboundMessage("task.status", map[string]any{"id": 42})
	after := time.Now().Unix()

	require.NotNil(t, msg)
	assert.Equal(t, "task.status", msg.Type)
	assert.Equal(t, map[string]any{"id": 42}, msg.Payload)
	assert.GreaterOrEqual(t, msg.Timestamp, before)
	assert.LessOrEqual(t, msg.Timestamp, after)
	assert.Empty(t, msg.ID)
	assert.Empty(t, msg.Error)
}

func TestNewOutboundMessage_NilPayload(t *testing.T) {
	t.Parallel()
	msg := NewOutboundMessage("ping", nil)

	require.NotNil(t, msg)
	assert.Equal(t, "ping", msg.Type)
	assert.Nil(t, msg.Payload)
	assert.NotZero(t, msg.Timestamp)
}

func TestNewErrorMessage(t *testing.T) {
	t.Parallel()
	before := time.Now().Unix()
	msg := NewErrorMessage("something broke")
	after := time.Now().Unix()

	require.NotNil(t, msg)
	assert.Equal(t, TypeError, msg.Type)

	payload, ok := msg.Payload.(ErrorPayload)
	require.True(t, ok, "payload must be ErrorPayload")
	assert.Equal(t, "something broke", payload.Message)

	assert.GreaterOrEqual(t, msg.Timestamp, before)
	assert.LessOrEqual(t, msg.Timestamp, after)
	assert.Empty(t, msg.ID)
	assert.Empty(t, msg.Error)
}

func TestOutboundMessage_Marshal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		msg            OutboundMessage
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "full_message",
			msg: OutboundMessage{
				Type:      "task.status",
				Payload:   map[string]any{"state": "running"},
				ID:        "req-1",
				Timestamp: 1700000000,
				Error:     "boom",
			},
			wantContains: []string{
				`"type":"task.status"`,
				`"payload":{"state":"running"}`,
				`"id":"req-1"`,
				`"ts":1700000000`,
				`"error":"boom"`,
			},
		},
		{
			name: "omits_empty_optional_fields",
			msg: OutboundMessage{
				Type:      "pong",
				Timestamp: 1700000000,
			},
			wantContains: []string{
				`"type":"pong"`,
				`"ts":1700000000`,
			},
			wantNotContain: []string{
				`"id"`,
				`"error"`,
				`"payload"`,
			},
		},
		{
			name: "empty_type_still_serialised",
			msg: OutboundMessage{
				Timestamp: 1,
			},
			wantContains: []string{
				`"type":""`,
				`"ts":1`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(tt.msg)
			require.NoError(t, err)

			s := string(data)
			for _, want := range tt.wantContains {
				assert.Contains(t, s, want)
			}
			for _, notWant := range tt.wantNotContain {
				assert.NotContains(t, s, notWant)
			}
		})
	}
}

func TestInboundMessage_Unmarshal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		input       string
		wantType    string
		wantID      string
		wantPayload string
		wantError   string
	}{
		{
			name:        "valid_envelope_with_payload",
			input:       `{"type":"task.subscribe","id":"req-1","payload":{"task_id":42}}`,
			wantType:    "task.subscribe",
			wantID:      "req-1",
			wantPayload: `{"task_id":42}`,
		},
		{
			name:     "minimal_envelope",
			input:    `{"type":"ping"}`,
			wantType: "ping",
		},
		{
			name:        "raw_array_payload",
			input:       `{"type":"console.input","payload":["one","two"]}`,
			wantType:    "console.input",
			wantPayload: `["one","two"]`,
		},
		{
			name:      "invalid_json",
			input:     `{not json`,
			wantError: "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var msg InboundMessage
			err := json.Unmarshal([]byte(tt.input), &msg)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantType, msg.Type)
			assert.Equal(t, tt.wantID, msg.ID)
			if tt.wantPayload == "" {
				assert.Empty(t, msg.Payload)
			} else {
				assert.JSONEq(t, tt.wantPayload, string(msg.Payload))
			}
		})
	}
}

func TestMessageTypes_Constants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "ping", TypePing)
	assert.Equal(t, "pong", TypePong)
	assert.Equal(t, "error", TypeError)
}
