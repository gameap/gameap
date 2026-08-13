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
	// ErrPluginNotInitialized means Register or Replace was handed something
	// that never came out of a successful load.
	ErrPluginNotInitialized = errors.New("plugin is not initialized")
)

var knownErrors = []error{
	ErrAPIVersionMismatch,
	ErrExportNotFound,
	ErrMemoryOutOfRange,
	ErrPluginReturnedError,
	ErrUnexpectedExitCode,
	ErrPluginAlreadyLoaded,
	ErrPluginNotFound,
}

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
