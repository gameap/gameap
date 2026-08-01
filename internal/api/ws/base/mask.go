// Package base holds helpers shared by the WebSocket API handlers.
package base

import (
	"bytes"
	"encoding/json"

	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/secretmask"
)

// outboundFrame mirrors ws.OutboundMessage with the payload left undecoded, so a frame can
// be taken apart and put back together without losing envelope fields.
type outboundFrame struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	ID        string          `json:"id,omitempty"`
	Timestamp int64           `json:"ts"`
	Error     string          `json:"error,omitempty"`
}

// NewOutboundMaskFilter builds a ws.OutboundFilter that replaces every secret known to
// masker with the placeholder before a frame reaches the browser.
//
// The two console-carrying payloads are masked on the decoded value, because their JSON
// form can hide the secret from a plain text scan: attach.output holds raw PTY bytes,
// which encode as base64, and a console.output chunk goes through JSON string escaping.
// The encoded frame is then masked as well, which covers error messages and any frame
// type added later.
//
// Returns nil when there is nothing to mask, so callers can skip installing a filter
// altogether.
func NewOutboundMaskFilter(masker *secretmask.Masker) ws.OutboundFilter {
	if masker.Empty() {
		return nil
	}

	return func(frame []byte) []byte {
		return masker.Bytes(maskPayload(frame, masker))
	}
}

// maskPayload rebuilds the frame around a masked payload. The frame is returned untouched
// when it carries no known payload, cannot be decoded, or held no secret to begin with —
// the caller still masks the encoded frame afterwards.
func maskPayload(frame []byte, masker *secretmask.Masker) []byte {
	var decoded outboundFrame
	if err := json.Unmarshal(frame, &decoded); err != nil {
		return frame
	}

	var (
		encodedPayload []byte
		err            error
	)

	switch decoded.Type {
	case messages.TypeAttachOutput:
		encodedPayload, err = maskAttachOutput(decoded.Payload, masker)
	case messages.TypeConsoleOutput:
		encodedPayload, err = maskConsoleOutput(decoded.Payload, masker)
	default:
		return frame
	}

	if err != nil || encodedPayload == nil {
		return frame
	}

	decoded.Payload = encodedPayload

	encodedFrame, err := json.Marshal(decoded)
	if err != nil {
		return frame
	}

	return encodedFrame
}

// maskAttachOutput returns the re-encoded payload, or a nil payload when nothing was
// masked.
func maskAttachOutput(rawPayload json.RawMessage, masker *secretmask.Masker) ([]byte, error) {
	var payload messages.AttachOutputPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, err
	}

	masked := masker.Bytes(payload.Data)
	if bytes.Equal(masked, payload.Data) {
		return nil, nil //nolint:nilnil // "nothing to mask" is not an error here
	}

	payload.Data = masked

	return json.Marshal(payload)
}

// maskConsoleOutput returns the re-encoded payload, or a nil payload when nothing was
// masked.
func maskConsoleOutput(rawPayload json.RawMessage, masker *secretmask.Masker) ([]byte, error) {
	var payload messages.ConsoleOutputPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, err
	}

	masked := masker.String(payload.Chunk)
	if masked == payload.Chunk {
		return nil, nil //nolint:nilnil // "nothing to mask" is not an error here
	}

	payload.Chunk = masked

	return json.Marshal(payload)
}
