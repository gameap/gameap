package plugin

import (
	"context"
	"log/slog"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// EnvHostLibrary provides the "env" module required by AssemblyScript plugins.
// It exports:
//   - abort(messagePtr, fileNamePtr, line, column) - called on runtime errors.
//   - console.log(messagePtr) - called for logging.
type EnvHostLibrary struct{}

// Instantiate registers the env host functions into the given runtime.
func (e *EnvHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	_, err := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(e.abort).
		WithParameterNames("messagePtr", "fileNamePtr", "line", "column").
		Export("abort").
		NewFunctionBuilder().
		WithFunc(e.consoleLog).
		WithParameterNames("messagePtr").
		Export("console.log").
		Instantiate(ctx)

	return err
}

// abort is called by AssemblyScript when an assertion fails or an error occurs.
// Parameters:
//   - messagePtr: pointer to the error message string.
//   - fileNamePtr: pointer to the filename string.
//   - line: line number where the error occurred.
//   - column: column number where the error occurred.
func (e *EnvHostLibrary) abort(_ context.Context, m api.Module, messagePtr, fileNamePtr, line, column uint32) {
	message := readAssemblyScriptString(m, messagePtr)
	fileName := readAssemblyScriptString(m, fileNamePtr)

	slog.Error("[Plugin] AssemblyScript abort",
		slog.String("message", message),
		slog.String("file", fileName),
		slog.Uint64("line", uint64(line)),
		slog.Uint64("column", uint64(column)),
	)
}

// consoleLog is called by AssemblyScript for console.log output.
// Parameters:
//   - messagePtr: pointer to the message string.
func (e *EnvHostLibrary) consoleLog(_ context.Context, m api.Module, messagePtr uint32) {
	message := readAssemblyScriptString(m, messagePtr)
	slog.Info("[Plugin]", slog.String("message", message))
}

// readAssemblyScriptString reads a string from AssemblyScript's memory.
// AssemblyScript managed objects have the following memory layout:
//
//	ptr-20: mmInfo (memory manager info)
//	ptr-16: gcInfo (garbage collector info)
//	ptr-12: gcInfo2
//	ptr-8:  rtId (runtime type id)
//	ptr-4:  rtSize (byte length of the data, NOT code units)
//	ptr:    data starts here
//
// For strings, rtSize is the byte length (already in bytes, not code units).
func readAssemblyScriptString(m api.Module, ptr uint32) string {
	if ptr == 0 {
		return ""
	}

	mem := m.Memory()
	if mem == nil {
		return "<no memory>"
	}

	// AssemblyScript stores byte length at ptr-4 (rtSize field)
	rtSizeBytes, ok := mem.Read(ptr-4, 4)
	if !ok {
		return "<invalid ptr>"
	}

	byteLength := uint32(rtSizeBytes[0]) | uint32(rtSizeBytes[1])<<8 |
		uint32(rtSizeBytes[2])<<16 | uint32(rtSizeBytes[3])<<24

	// Sanity check to avoid reading huge amounts of memory
	if byteLength > 1024*1024 {
		return "<string too large>"
	}

	data, ok := mem.Read(ptr, byteLength)
	if !ok {
		return "<read error>"
	}

	// Convert UTF-16LE to string
	return utf16LEToString(data)
}

// utf16LEToString converts UTF-16LE encoded bytes to a Go string.
func utf16LEToString(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	// Simple conversion for ASCII-compatible characters
	// For full UTF-16 support, use unicode/utf16 package
	// i+1 bound guards against an odd byteLength read from guest memory.
	result := make([]byte, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		low := data[i]
		high := data[i+1]

		if high == 0 {
			// ASCII character
			result = append(result, low)
		} else {
			// Non-ASCII: use replacement character or proper UTF-16 decoding
			codePoint := uint16(low) | uint16(high)<<8

			//nolint:gosec // G115: bitwise operations guarantee byte range (0-255)
			switch {
			case codePoint < 0x80:
				result = append(result, byte(codePoint))
			case codePoint < 0x800:
				result = append(result,
					byte(0xC0|(codePoint>>6)),
					byte(0x80|(codePoint&0x3F)),
				)
			default:
				result = append(result,
					byte(0xE0|(codePoint>>12)),
					byte(0x80|((codePoint>>6)&0x3F)),
					byte(0x80|(codePoint&0x3F)),
				)
			}
		}
	}

	return string(result)
}
