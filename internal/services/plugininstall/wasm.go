package plugininstall

import (
	"io"
	"net/http"

	"github.com/pkg/errors"

	"github.com/gameap/gameap/pkg/api"
)

const (
	MaxMemory     = 32 << 20  // 32 MB
	MaxUploadSize = 100 << 20 // 100 MB
)

var (
	ErrNoFileUploaded   = errors.New("no file uploaded")
	ErrFileTooSmall     = errors.New("file too small to be valid WASM")
	ErrInvalidWASMMagic = errors.New("invalid WASM magic number")
)

// ValidateWASM rejects anything that is not a WebAssembly module. The errors
// carry a 400: uploading the wrong file is the caller's mistake, and a 500
// would hide the reason behind "Internal Server Error".
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

	if err := r.ParseMultipartForm(MaxMemory); err != nil {
		return nil, errors.WithMessage(err, "failed to parse multipart form")
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		return nil, ErrNoFileUploaded
	}
	defer func() { _ = file.Close() }()

	wasmBytes, err := io.ReadAll(file)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read uploaded file")
	}

	return wasmBytes, nil
}
