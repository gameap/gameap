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
// Known payloads are masked on the decoded value, because their JSON form can hide the
// secret from a plain text scan: attach.output holds raw PTY bytes, which encode as
// base64, while console.output chunks and error messages go through JSON string escaping.
// The encoded frame is then masked as well, which covers any frame type added later.
//
// Returns nil when there is nothing to mask, so callers can skip installing a filter
// altogether.
func NewOutboundMaskFilter(masker *secretmask.Masker) ws.OutboundFilter {
	if masker.Empty() {
		return nil
	}

	return func(frame []byte) []byte {
		return masker.Bytes(maskFrame(frame, masker))
	}
}

// maskFrame rebuilds the frame around a masked envelope and payload. The frame is returned
// untouched when it cannot be decoded or held no secret to begin with — the caller still
// masks the encoded frame afterwards.
func maskFrame(frame []byte, masker *secretmask.Masker) []byte {
	var decoded outboundFrame
	if err := json.Unmarshal(frame, &decoded); err != nil {
		return frame
	}

	// The envelope's own error text is masked for every frame type, not just error frames.
	maskedError := masker.String(decoded.Error)

	var encodedPayload []byte

	switch decoded.Type {
	case messages.TypeAttachOutput:
		encodedPayload = maskAttachOutput(decoded.Payload, masker)
	case messages.TypeConsoleOutput:
		encodedPayload = maskConsoleOutput(decoded.Payload, masker)
	case ws.TypeError:
		encodedPayload = maskErrorMessage(decoded.Payload, masker)
	}

	if encodedPayload == nil && maskedError == decoded.Error {
		return frame
	}

	if encodedPayload != nil {
		decoded.Payload = encodedPayload
	}

	decoded.Error = maskedError

	encodedFrame, err := json.Marshal(decoded)
	if err != nil {
		return frame
	}

	return encodedFrame
}

// maskAttachOutput returns the re-encoded payload, or nil when the payload could not be
// decoded or carried no secret. A payload left alone is still covered by the caller's
// mask over the encoded frame.
func maskAttachOutput(rawPayload json.RawMessage, masker *secretmask.Masker) []byte {
	var payload messages.AttachOutputPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil
	}

	masked := masker.Bytes(payload.Data)
	if bytes.Equal(masked, payload.Data) {
		return nil
	}

	payload.Data = masked

	return marshalPayload(payload)
}

// maskConsoleOutput returns the re-encoded payload, or nil when the payload could not be
// decoded or carried no secret.
func maskConsoleOutput(rawPayload json.RawMessage, masker *secretmask.Masker) []byte {
	var payload messages.ConsoleOutputPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil
	}

	masked := masker.String(payload.Chunk)
	if masked == payload.Chunk {
		return nil
	}

	payload.Chunk = masked

	return marshalPayload(payload)
}

// maskErrorMessage returns the re-encoded payload, or nil when the payload could not be
// decoded or carried no secret.
func maskErrorMessage(rawPayload json.RawMessage, masker *secretmask.Masker) []byte {
	var payload ws.ErrorPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil
	}

	masked := masker.String(payload.Message)
	if masked == payload.Message {
		return nil
	}

	payload.Message = masked

	return marshalPayload(payload)
}

func marshalPayload(payload any) []byte {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	return encoded
}
