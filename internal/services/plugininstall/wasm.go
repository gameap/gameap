package plugininstall

import (
	"io"
	"net/http"

	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

const (
	MaxMemory     = 32 << 20  // 32 MB
	MaxUploadSize = 100 << 20 // 100 MB
)

// The three below describe a request the caller got wrong, so they travel as
// 400: a 5xx would put a mistyped upload in the error log and tell the client
// to retry something that can only be fixed by picking another file.
var (
	ErrNoFileUploaded   = errors.New("no file uploaded")
	ErrFileTooSmall     = errors.New("file too small to be valid WASM")
	ErrInvalidWASMMagic = errors.New("invalid WASM magic number")
)

func ValidateWASM(data []byte) error {
	if len(data) < 4 {
		return api.WrapHTTPError(ErrFileTooSmall, http.StatusBadRequest)
	}
	// WASM magic number: \x00asm
	if data[0] != 0x00 || data[1] != 0x61 || data[2] != 0x73 || data[3] != 0x6d {
		return api.WrapHTTPError(ErrInvalidWASMMagic, http.StatusBadRequest)
	}

	return nil
}

func ReadWASMFromMultipart(rw http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(rw, r.Body, MaxUploadSize)

	// MaxMemory only decides how much of the form stays in RAM before it
	// spills to a temp file; the request size is bounded by MaxBytesReader.
	//nolint:gosec // G120: r.Body is capped by MaxUploadSize above
	if err := r.ParseMultipartForm(MaxMemory); err != nil {
		return nil, errors.WithMessage(err, "failed to parse multipart form")
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		return nil, api.WrapHTTPError(ErrNoFileUploaded, http.StatusBadRequest)
	}
	defer func() { _ = file.Close() }()

	wasmBytes, err := io.ReadAll(file)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read uploaded file")
	}

	return wasmBytes, nil
}
