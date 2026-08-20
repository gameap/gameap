package base_test

import (
	"encoding/json"
	"testing"

	wsbase "github.com/gameap/gameap/internal/api/ws/base"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/secretmask"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRconPassword = "s3cr3tRc0n"

func encodeFrame(t *testing.T, msgType string, payload any) []byte {
	t.Helper()

	encodedPayload, err := json.Marshal(payload)
	require.NoError(t, err)

	frame, err := json.Marshal(ws.NewOutboundMessage(msgType, json.RawMessage(encodedPayload)))
	require.NoError(t, err)

	return frame
}

func TestNewOutboundMaskFilter_returns_nil_for_empty_masker(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		masker *secretmask.Masker
	}{
		{name: "nil_masker", masker: nil},
		{name: "masker_without_secrets", masker: secretmask.New()},
		{name: "masker_with_empty_secret", masker: secretmask.New("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			filter := wsbase.NewOutboundMaskFilter(tt.masker)

			// ASSERT
			assert.Nil(t, filter, "no secret means no filter should be installed")
		})
	}
}

func TestNewOutboundMaskFilter_masksAttachOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		data     string
		wantData string
	}{
		{
			name:     "launch_line_with_password_is_masked",
			data:     "./hlds_run -game cs +rcon_password " + testRconPassword + "\r\n",
			wantData: "./hlds_run -game cs +rcon_password ******\r\n",
		},
		{
			name:     "every_occurrence_is_masked",
			data:     testRconPassword + " " + testRconPassword,
			wantData: "****** ******",
		},
		{
			name:     "chunk_without_password_is_untouched",
			data:     "L 01/02/2026 - 12:00:00: Started map \"de_dust2\"\r\n",
			wantData: "L 01/02/2026 - 12:00:00: Started map \"de_dust2\"\r\n",
		},
		{
			name:     "empty_chunk_is_untouched",
			data:     "",
			wantData: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			filter := wsbase.NewOutboundMaskFilter(secretmask.New(testRconPassword))
			require.NotNil(t, filter)

			frame := encodeFrame(t, messages.TypeAttachOutput, messages.AttachOutputPayload{
				SessionID: "session-1",
				Data:      []byte(tt.data),
			})

			// ACT
			masked := filter(frame)

			// ASSERT
			var decoded struct {
				Type    string                       `json:"type"`
				Payload messages.AttachOutputPayload `json:"payload"`
				Ts      int64                        `json:"ts"`
			}
			require.NoError(t, json.Unmarshal(masked, &decoded))

			assert.Equal(t, messages.TypeAttachOutput, decoded.Type)
			assert.Equal(t, "session-1", decoded.Payload.SessionID)
			assert.Equal(t, tt.wantData, string(decoded.Payload.Data))
			assert.NotContains(t, string(masked), testRconPassword)
		})
	}
}

func TestNewOutboundMaskFilter_masksConsoleOutputChunk(t *testing.T) {
	t.Parallel()
	// ARRANGE
	filter := wsbase.NewOutboundMaskFilter(secretmask.New(testRconPassword))
	require.NotNil(t, filter)

	frame := encodeFrame(t, messages.TypeConsoleOutput, messages.ConsoleOutputPayload{
		ServerID: 33,
		Chunk:    "+rcon_password " + testRconPassword,
	})

	// ACT
	masked := filter(frame)

	// ASSERT
	var decoded struct {
		Type    string                        `json:"type"`
		Payload messages.ConsoleOutputPayload `json:"payload"`
	}
	require.NoError(t, json.Unmarshal(masked, &decoded))

	assert.Equal(t, messages.TypeConsoleOutput, decoded.Type)
	assert.Equal(t, uint64(33), decoded.Payload.ServerID)
	assert.Equal(t, "+rcon_password ******", decoded.Payload.Chunk)
}

// A password holding characters that JSON escapes ("\ and friends) never appears verbatim
// in the encoded frame, so masking it depends on the payload being decoded first.
func TestNewOutboundMaskFilter_masksJSONEscapedPassword(t *testing.T) {
	t.Parallel()
	const escapedPassword = `pa"ss\word`

	tests := []struct {
		name      string
		frameType string
		payload   any
		wantValue string
		readValue func(t *testing.T, payload json.RawMessage) string
	}{
		{
			name:      "attach_output",
			frameType: messages.TypeAttachOutput,
			payload: messages.AttachOutputPayload{
				SessionID: "session-1",
				Data:      []byte("+rcon_password " + escapedPassword),
			},
			wantValue: "+rcon_password ******",
			readValue: func(t *testing.T, payload json.RawMessage) string {
				t.Helper()

				var decoded messages.AttachOutputPayload
				require.NoError(t, json.Unmarshal(payload, &decoded))

				return string(decoded.Data)
			},
		},
		{
			name:      "console_output",
			frameType: messages.TypeConsoleOutput,
			payload: messages.ConsoleOutputPayload{
				ServerID: 33,
				Chunk:    "+rcon_password " + escapedPassword,
			},
			wantValue: "+rcon_password ******",
			readValue: func(t *testing.T, payload json.RawMessage) string {
				t.Helper()

				var decoded messages.ConsoleOutputPayload
				require.NoError(t, json.Unmarshal(payload, &decoded))

				return decoded.Chunk
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			filter := wsbase.NewOutboundMaskFilter(secretmask.New(escapedPassword))
			require.NotNil(t, filter)

			frame := encodeFrame(t, tt.frameType, tt.payload)
			require.NotContains(t, string(frame), escapedPassword,
				"the escaped form is what makes this case interesting")

			// ACT
			masked := filter(frame)

			// ASSERT
			var decoded struct {
				Type    string          `json:"type"`
				Payload json.RawMessage `json:"payload"`
			}
			require.NoError(t, json.Unmarshal(masked, &decoded))

			assert.Equal(t, tt.frameType, decoded.Type)
			assert.Equal(t, tt.wantValue, tt.readValue(t, decoded.Payload))
		})
	}
}

func TestNewOutboundMaskFilter_masksUnknownFrameTypes(t *testing.T) {
	t.Parallel()
	// ARRANGE
	filter := wsbase.NewOutboundMaskFilter(secretmask.New(testRconPassword))
	require.NotNil(t, filter)

	frame := encodeFrame(t, "console.history", map[string]string{
		"output": "+rcon_password " + testRconPassword,
	})

	// ACT
	masked := filter(frame)

	// ASSERT
	assert.NotContains(t, string(masked), testRconPassword)
	assert.Contains(t, string(masked), "+rcon_password ******")
}

func TestNewOutboundMaskFilter_masksErrorFrames(t *testing.T) {
	t.Parallel()
	// ARRANGE
	filter := wsbase.NewOutboundMaskFilter(secretmask.New(testRconPassword))
	require.NotNil(t, filter)

	frame, err := json.Marshal(ws.NewErrorMessage("rcon auth failed for " + testRconPassword))
	require.NoError(t, err)

	// ACT
	masked := filter(frame)

	// ASSERT
	assert.NotContains(t, string(masked), testRconPassword)
	assert.Contains(t, string(masked), "rcon auth failed for ******")
}

func TestNewOutboundMaskFilter_masksJSONEscapedPasswordInErrorFrame(t *testing.T) {
	t.Parallel()
	const escapedPassword = `pa"ss\word`

	// ARRANGE
	filter := wsbase.NewOutboundMaskFilter(secretmask.New(escapedPassword))
	require.NotNil(t, filter)

	frame, err := json.Marshal(ws.NewErrorMessage("rcon auth failed for " + escapedPassword))
	require.NoError(t, err)
	require.NotContains(t, string(frame), escapedPassword,
		"the escaped form is what makes this case interesting")

	// ACT
	masked := filter(frame)

	// ASSERT
	var decoded struct {
		Type    string          `json:"type"`
		Payload ws.ErrorPayload `json:"payload"`
	}
	require.NoError(t, json.Unmarshal(masked, &decoded))

	assert.Equal(t, ws.TypeError, decoded.Type)
	assert.Equal(t, "rcon auth failed for ******", decoded.Payload.Message)
}

func TestNewOutboundMaskFilter_masksEnvelopeErrorField(t *testing.T) {
	t.Parallel()
	const escapedPassword = `pa"ss\word`

	// ARRANGE
	filter := wsbase.NewOutboundMaskFilter(secretmask.New(escapedPassword))
	require.NotNil(t, filter)

	frame, err := json.Marshal(&ws.OutboundMessage{
		Type:      messages.TypeConsoleOutput,
		Timestamp: 1700000000,
		Error:     "command rejected: " + escapedPassword,
	})
	require.NoError(t, err)

	// ACT
	masked := filter(frame)

	// ASSERT
	var decoded struct {
		Type  string `json:"type"`
		Error string `json:"error"`
		Ts    int64  `json:"ts"`
	}
	require.NoError(t, json.Unmarshal(masked, &decoded))

	assert.Equal(t, "command rejected: ******", decoded.Error)
	assert.Equal(t, int64(1700000000), decoded.Ts)
}

func TestNewOutboundMaskFilter_masksMalformedFrames(t *testing.T) {
	t.Parallel()
	// ARRANGE
	filter := wsbase.NewOutboundMaskFilter(secretmask.New(testRconPassword))
	require.NotNil(t, filter)

	frame := []byte("this is not json but contains " + testRconPassword)

	// ACT
	masked := filter(frame)

	// ASSERT
	assert.Equal(t, "this is not json but contains ******", string(masked))
}

func TestNewOutboundMaskFilter_preservesEnvelopeFields(t *testing.T) {
	t.Parallel()
	// ARRANGE
	filter := wsbase.NewOutboundMaskFilter(secretmask.New(testRconPassword))
	require.NotNil(t, filter)

	encodedPayload, err := json.Marshal(messages.AttachOutputPayload{
		SessionID: "session-1",
		Data:      []byte("+rcon_password " + testRconPassword),
	})
	require.NoError(t, err)

	original := &ws.OutboundMessage{
		Type:      messages.TypeAttachOutput,
		Payload:   json.RawMessage(encodedPayload),
		ID:        "msg-7",
		Timestamp: 1700000000,
	}
	frame, err := json.Marshal(original)
	require.NoError(t, err)

	// ACT
	masked := filter(frame)

	// ASSERT
	var decoded struct {
		Type    string                       `json:"type"`
		Payload messages.AttachOutputPayload `json:"payload"`
		ID      string                       `json:"id"`
		Ts      int64                        `json:"ts"`
	}
	require.NoError(t, json.Unmarshal(masked, &decoded))

	assert.Equal(t, messages.TypeAttachOutput, decoded.Type)
	assert.Equal(t, "msg-7", decoded.ID)
	assert.Equal(t, int64(1700000000), decoded.Ts)
	assert.Equal(t, "+rcon_password ******", string(decoded.Payload.Data))
}

func TestNewOutboundMaskFilter_returnsFrameAsIsWhenNothingMatches(t *testing.T) {
	t.Parallel()
	// ARRANGE
	filter := wsbase.NewOutboundMaskFilter(secretmask.New(testRconPassword))
	require.NotNil(t, filter)

	frame := encodeFrame(t, messages.TypeAttachOutput, messages.AttachOutputPayload{
		SessionID: "session-1",
		Data:      []byte("nothing to hide"),
	})

	// ACT
	masked := filter(frame)

	// ASSERT
	require.Len(t, masked, len(frame))
	assert.Equal(t, &frame[0], &masked[0], "untouched frames must not be re-encoded")
}
