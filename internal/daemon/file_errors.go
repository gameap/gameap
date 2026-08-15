package daemon

import (
	"net/http"
	"strings"

	"github.com/pkg/errors"
)

// fileStatusError is a daemon-reported filesystem failure kind. It carries the
// HTTP status the API layer should answer with, following daemonNotConnectedError.
type fileStatusError struct {
	msg    string
	status int
}

func (e *fileStatusError) Error() string   { return e.msg }
func (e *fileStatusError) HTTPStatus() int { return e.status }

var (
	ErrFileNotFound      error = &fileStatusError{"file not found", http.StatusNotFound}
	ErrPermissionDenied  error = &fileStatusError{"permission denied", http.StatusForbidden}
	ErrIsDirectory       error = &fileStatusError{"path is a directory", http.StatusBadRequest}
	ErrNotDirectory      error = &fileStatusError{"path is not a directory", http.StatusBadRequest}
	ErrFileExists        error = &fileStatusError{"file already exists", http.StatusConflict}
	ErrDirectoryNotEmpty error = &fileStatusError{"directory is not empty", http.StatusConflict}
	ErrFileBusy          error = &fileStatusError{"file is busy", http.StatusConflict}
	ErrFileTooLarge      error = &fileStatusError{"file too large", http.StatusRequestEntityTooLarge}
	ErrNoSpaceLeft       error = &fileStatusError{"no space left on device", http.StatusInsufficientStorage}
	ErrInvalidPath       error = &fileStatusError{"invalid path", http.StatusBadRequest}
)

// FileError is a daemon-side filesystem failure with a recognised cause, in the
// spirit of *os.PathError. Error() deliberately stays free of the raw daemon
// text: that text names paths relative to the node work directory (server dir
// included), which API clients must not see. Detail keeps it for logs.
type FileError struct {
	// Op is what the panel was doing: "stat", "file list", "delete", "upload task", ...
	Op string
	// Err is one of the sentinels above (ErrFileNotFound, ErrPermissionDenied, ...).
	Err error
	// Detail is the raw daemon error message.
	Detail string
}

func (e *FileError) Error() string {
	if e.Err == nil {
		return "file operation failed"
	}

	return e.Err.Error()
}

func (e *FileError) Unwrap() error { return e.Err }

func (e *FileError) HTTPStatus() int {
	var statusErr interface{ HTTPStatus() int }
	if errors.As(e.Err, &statusErr) {
		return statusErr.HTTPStatus()
	}

	return http.StatusInternalServerError
}

// Description feeds the API responder's log line (see pkg/api Responder.WriteError),
// so operators still get the daemon's own words while clients get Error().
func (e *FileError) Description() string { return e.Op + ": " + e.Detail }

// fileErrorPatterns maps substrings of daemon error text to a failure kind.
// The daemon forwards raw *os.PathError messages, so both Go/POSIX and Windows
// wordings are listed. Order matters where one phrase contains another.
var fileErrorPatterns = []struct {
	substr string
	err    error
}{
	{"path is outside work directory", ErrInvalidPath},
	{"path escapes from parent", ErrInvalidPath},
	{"file name too long", ErrInvalidPath},
	{"not a directory", ErrNotDirectory},
	{"is a directory", ErrIsDirectory},
	{"no such file or directory", ErrFileNotFound},
	{"file does not exist", ErrFileNotFound},
	{"cannot find the file specified", ErrFileNotFound},
	{"cannot find the path specified", ErrFileNotFound},
	{"permission denied", ErrPermissionDenied},
	{"operation not permitted", ErrPermissionDenied},
	{"access is denied", ErrPermissionDenied},
	{"file exists", ErrFileExists},
	{"already exists", ErrFileExists},
	{"directory not empty", ErrDirectoryNotEmpty},
	{"directory is not empty", ErrDirectoryNotEmpty},
	{"text file busy", ErrFileBusy},
	{"file too large", ErrFileTooLarge},
	{"no space left on device", ErrNoSpaceLeft},
	{"not enough space on the disk", ErrNoSpaceLeft},
	{"disk quota exceeded", ErrNoSpaceLeft},
}

// workDirUnavailableMarker is what the daemon prefixes when its own work
// directory cannot be opened. That is a node misconfiguration, not a client
// mistake, so such messages must keep surfacing as an internal error even
// though they contain "no such file or directory" or "permission denied".
const workDirUnavailableMarker = "work directory unavailable"

// daemonFileError classifies a failure message reported by the daemon.
// It returns nil when the message is not a recognised filesystem failure so
// callers can keep their existing opaque error.
func daemonFileError(op, msg string) error {
	lower := strings.ToLower(msg)
	if strings.Contains(lower, workDirUnavailableMarker) {
		return nil
	}

	for _, p := range fileErrorPatterns {
		if strings.Contains(lower, p.substr) {
			return &FileError{Op: op, Err: p.err, Detail: msg}
		}
	}

	return nil
}

// classifyFileError is daemonFileError for error values coming back from the
// gateway or the dispatcher, whose text is the daemon message verbatim.
// Unrecognised errors are returned unchanged.
func classifyFileError(op string, err error) error {
	if err == nil {
		return nil
	}

	if ferr := daemonFileError(op, err.Error()); ferr != nil {
		return ferr
	}

	return err
}
