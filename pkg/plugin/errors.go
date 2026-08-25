package plugin

import (
	"strings"

	"github.com/pkg/errors"
)

var (
	ErrManagerClosed        = errors.New("plugin manager is closed")
	ErrPluginAlreadyLoaded  = errors.New("plugin already loaded")
	ErrPluginNotFound       = errors.New("plugin not found")
	ErrAPIVersionMismatch   = errors.New("API version mismatch")
	ErrUnexpectedExitCode   = errors.New("unexpected exit code")
	ErrInitializationFailed = errors.New("plugin initialization failed")
	ErrExportNotFound       = errors.New("required export not found")
	ErrMemoryOutOfRange     = errors.New("memory operation out of range")
	ErrPluginReturnedError  = errors.New("plugin returned error")
	// ErrPluginBusy means the caller gave up waiting for the per-plugin call
	// gate; the guest was never invoked and its module is untouched.
	ErrPluginBusy = errors.New("plugin is busy")
	// ErrModuleTooLarge rejects a wasm file above ManagerConfig.MaxModuleBytes
	// before any compilation work.
	ErrModuleTooLarge = errors.New("plugin module too large")
)

var knownErrors = []error{
	ErrAPIVersionMismatch,
	ErrExportNotFound,
	ErrMemoryOutOfRange,
	ErrPluginReturnedError,
	ErrUnexpectedExitCode,
	ErrPluginAlreadyLoaded,
	ErrPluginNotFound,
	ErrModuleTooLarge,
}

// memoryLimitErrorMarker is the wazero decoder's wording for a module whose
// declared memory exceeds the runtime limit; the message names both sizes
// and carries no stack trace, so it is safe to show as is.
const memoryLimitErrorMarker = "over limit of"

// SanitizeLoadError processes a plugin loading error and returns a sanitized version.
// For known plugin errors, it returns the original error.
// For WASM runtime errors with stack traces, it removes the Go runtime stack trace.
// For other errors, it returns a generic message.
func SanitizeLoadError(err error) error {
	if err == nil {
		return nil
	}

	for _, knownErr := range knownErrors {
		if errors.Is(err, knownErr) {
			return err
		}
	}

	errMsg := err.Error()

	if strings.Contains(errMsg, memoryLimitErrorMarker) {
		return err
	}

	if strings.Contains(errMsg, "wasm stack trace:") {
		before, _, found := strings.Cut(errMsg, "Go runtime stack trace:")
		if found {
			sanitized := strings.TrimRight(before, "\n\t ")

			return errors.New(sanitized)
		}

		return err
	}

	return errors.New("failed to load plugin")
}
