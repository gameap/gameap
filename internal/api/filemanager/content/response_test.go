package content

import (
	"encoding/json"
	"testing"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewContentResponse_Mode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fileType daemon.FileType
		perm     uint32
		wantMode uint32
	}{
		{name: "file_mode_0o644", fileType: daemon.FileTypeFile, perm: 0o644, wantMode: 0o644},
		{name: "file_mode_0o755", fileType: daemon.FileTypeFile, perm: 0o755, wantMode: 0o755},
		{name: "file_mode_0o600", fileType: daemon.FileTypeFile, perm: 0o600, wantMode: 0o600},
		{name: "file_mode_0o000", fileType: daemon.FileTypeFile, perm: 0o000, wantMode: 0o000},
		{name: "file_mode_0o777", fileType: daemon.FileTypeFile, perm: 0o777, wantMode: 0o777},
		{name: "directory_mode_0o755", fileType: daemon.FileTypeDir, perm: 0o755, wantMode: 0o755},
		{name: "directory_mode_0o700", fileType: daemon.FileTypeDir, perm: 0o700, wantMode: 0o700},
		// Mask-bit cases: Perm carries syscall.Mode_t-style high bits
		// (file type, setuid, setgid, sticky). The response must strip
		// everything above the 9 standard rwxrwxrwx bits.
		{name: "file_high_bits_S_IFREG_stripped", fileType: daemon.FileTypeFile, perm: 0o100644, wantMode: 0o644},
		{name: "directory_high_bits_S_IFDIR_stripped", fileType: daemon.FileTypeDir, perm: 0o040755, wantMode: 0o755},
		{name: "file_setuid_bit_stripped", fileType: daemon.FileTypeFile, perm: 0o4755, wantMode: 0o755},
		{name: "file_setgid_bit_stripped", fileType: daemon.FileTypeFile, perm: 0o2755, wantMode: 0o755},
		{name: "file_sticky_bit_stripped", fileType: daemon.FileTypeFile, perm: 0o1755, wantMode: 0o755},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fileInfo := &daemon.FileInfo{
				Name:         "thing",
				Size:         42,
				TimeModified: 1759648383,
				Type:         tt.fileType,
				Perm:         tt.perm,
			}

			response := newContentResponse([]*daemon.FileInfo{fileInfo}, ".")

			switch tt.fileType {
			case daemon.FileTypeDir:
				require.Len(t, response.Directories, 1)
				require.Len(t, response.Files, 0)
				assert.Equal(t, tt.wantMode, response.Directories[0].Mode)
			case daemon.FileTypeFile:
				require.Len(t, response.Files, 1)
				require.Len(t, response.Directories, 0)
				assert.Equal(t, tt.wantMode, response.Files[0].Mode)
			default:
				t.Fatalf("unexpected file type in test fixture: %v", tt.fileType)
			}
		})
	}
}

func TestNewContentResponse_Mode_JSONShape(t *testing.T) {
	t.Parallel()

	// Locks the JSON wire contract: the field must be named "mode" and
	// serialized as a JSON number (uint32). Frontend code reads `item.mode`,
	// so a rename here would silently break the file manager.

	fileInfo := &daemon.FileInfo{
		Name:         "thing.txt",
		Size:         42,
		TimeModified: 1759648383,
		Type:         daemon.FileTypeFile,
		Perm:         0o644,
	}
	dirInfo := &daemon.FileInfo{
		Name:         "logs",
		Size:         0,
		TimeModified: 1759648383,
		Type:         daemon.FileTypeDir,
		Perm:         0o755,
	}

	response := newContentResponse([]*daemon.FileInfo{dirInfo, fileInfo}, ".")

	rawJSON, err := json.Marshal(response)
	require.NoError(t, err)

	var decoded struct {
		Directories []map[string]any `json:"directories"`
		Files       []map[string]any `json:"files"`
	}
	require.NoError(t, json.Unmarshal(rawJSON, &decoded))

	require.Len(t, decoded.Directories, 1)
	dirMode, ok := decoded.Directories[0]["mode"]
	require.True(t, ok, `directory item must include a "mode" key`)
	assert.InDelta(t, 0o755, dirMode, 0)

	require.Len(t, decoded.Files, 1)
	fileMode, ok := decoded.Files[0]["mode"]
	require.True(t, ok, `file item must include a "mode" key`)
	assert.InDelta(t, 0o644, fileMode, 0)
}
