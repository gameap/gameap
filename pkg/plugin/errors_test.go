package plugin

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestSanitizeLoadError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     error
		wantError string
	}{
		{
			name:      "nil_error",
			input:     nil,
			wantError: "",
		},
		{
			name: "wasm_error_with_go_stack_trace",
			//nolint:revive // this simulates external WASM runtime error format
			input:     errors.New("failed to call api_version: runtime error\nwasm stack trace:\n\tfunc1()\n\tfunc2()\nGo runtime stack trace:\ngoroutine 1 [running]:\n..."),
			wantError: "failed to call api_version: runtime error\nwasm stack trace:\n\tfunc1()\n\tfunc2()",
		},
		{
			name:      "wasm_error_without_go_stack_trace",
			input:     errors.New("failed to call api_version\nwasm stack trace:\n\tfunc1()"),
			wantError: "failed to call api_version\nwasm stack trace:\n\tfunc1()",
		},
		{
			name:      "non_wasm_error",
			input:     errors.New("connection refused"),
			wantError: "failed to load plugin",
		},
		{
			name:      "unknown_error_with_similar_text",
			input:     errors.New("api version mismatch: host=1, plugin=2"),
			wantError: "failed to load plugin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := SanitizeLoadError(tt.input)
			if tt.wantError == "" {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.wantError, result.Error())
			}
		})
	}
}

func TestSanitizeLoadError_known_errors(t *testing.T) {
	t.Parallel()
	knownErrors := []error{
		ErrAPIVersionMismatch,
		ErrExportNotFound,
		ErrMemoryOutOfRange,
		ErrPluginReturnedError,
		ErrUnexpectedExitCode,
		ErrPluginAlreadyLoaded,
		ErrPluginNotFound,
	}

	for _, knownErr := range knownErrors {
		t.Run(knownErr.Error(), func(t *testing.T) {
			t.Parallel()
			result := SanitizeLoadError(knownErr)
			assert.ErrorIs(t, result, knownErr)
		})
	}
}

func TestSanitizeLoadError_wrapped_known_errors(t *testing.T) {
	t.Parallel()
	knownErrors := []error{
		ErrAPIVersionMismatch,
		ErrExportNotFound,
		ErrMemoryOutOfRange,
		ErrPluginReturnedError,
		ErrUnexpectedExitCode,
		ErrPluginAlreadyLoaded,
		ErrPluginNotFound,
	}

	for _, knownErr := range knownErrors {
		t.Run("wrapped_"+knownErr.Error(), func(t *testing.T) {
			t.Parallel()
			wrapped := errors.Wrap(knownErr, "some context")
			result := SanitizeLoadError(wrapped)
			assert.ErrorIs(t, result, knownErr)
		})
	}
}
