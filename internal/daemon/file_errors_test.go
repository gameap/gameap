package daemon

import (
	"net/http"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemonFileError_classification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		msg        string
		wantErr    error
		wantStatus int
	}{
		{
			name:       "linux_enoent_from_lstat",
			msg:        "lstatat servers/q2/baseq2/server.cfg: no such file or directory",
			wantErr:    ErrFileNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "go_fs_err_not_exist",
			msg:        "open cfg/server.cfg: file does not exist",
			wantErr:    ErrFileNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "windows_file_not_found",
			msg:        `lstatat servers\q2\server.cfg: The system cannot find the file specified.`,
			wantErr:    ErrFileNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "windows_path_not_found",
			msg:        `openat servers\q2\missing\x.cfg: The system cannot find the path specified.`,
			wantErr:    ErrFileNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "linux_eacces",
			msg:        "openat servers/q2/secret.cfg: permission denied",
			wantErr:    ErrPermissionDenied,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "linux_eperm",
			msg:        "chmod servers/q2/bin: operation not permitted",
			wantErr:    ErrPermissionDenied,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "windows_access_denied",
			msg:        `openat servers\q2\locked.cfg: Access is denied.`,
			wantErr:    ErrPermissionDenied,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "daemon_read_of_directory",
			msg:        "path is a directory",
			wantErr:    ErrIsDirectory,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "linux_eisdir",
			msg:        "read servers/q2/baseq2: is a directory",
			wantErr:    ErrIsDirectory,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "daemon_list_of_regular_file",
			msg:        `path "servers/q2/pak0.pak" is not a directory`,
			wantErr:    ErrNotDirectory,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "linux_enotdir",
			msg:        "openat servers/q2/pak0.pak/x: not a directory",
			wantErr:    ErrNotDirectory,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "linux_eexist",
			msg:        "mkdirat servers/q2/baseq2: file exists",
			wantErr:    ErrFileExists,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "windows_already_exists",
			msg:        `mkdirat servers\q2\baseq2: Cannot create a file when that file already exists.`,
			wantErr:    ErrFileExists,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "linux_enotempty",
			msg:        "unlinkat servers/q2/baseq2: directory not empty",
			wantErr:    ErrDirectoryNotEmpty,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "windows_dir_not_empty",
			msg:        `unlinkat servers\q2\baseq2: The directory is not empty.`,
			wantErr:    ErrDirectoryNotEmpty,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "linux_etxtbsy",
			msg:        "openat servers/q2/q2ded: text file busy",
			wantErr:    ErrFileBusy,
			wantStatus: http.StatusConflict,
		},
		{
			name: "windows_sharing_violation",
			msg: `openat servers\q2\q2ded.exe: The process cannot access the file ` +
				`because it is being used by another process.`,
			wantErr:    ErrFileBusy,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "daemon_file_too_large",
			msg:        "file too large",
			wantErr:    ErrFileTooLarge,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "linux_enospc",
			msg:        "write servers/q2/pak9.pak: no space left on device",
			wantErr:    ErrNoSpaceLeft,
			wantStatus: http.StatusInsufficientStorage,
		},
		{
			name:       "windows_not_enough_space",
			msg:        `write servers\q2\pak9.pak: There is not enough space on the disk.`,
			wantErr:    ErrNoSpaceLeft,
			wantStatus: http.StatusInsufficientStorage,
		},
		{
			name:       "linux_edquot",
			msg:        "write servers/q2/pak9.pak: disk quota exceeded",
			wantErr:    ErrNoSpaceLeft,
			wantStatus: http.StatusInsufficientStorage,
		},
		{
			name:       "daemon_path_outside_work_directory",
			msg:        "path is outside work directory",
			wantErr:    ErrInvalidPath,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "os_root_symlink_escape",
			msg:        "openat servers/q2/link/x.cfg: path escapes from parent",
			wantErr:    ErrInvalidPath,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "linux_enametoolong",
			msg:        "openat servers/q2/aaaa...: file name too long",
			wantErr:    ErrInvalidPath,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "matching_is_case_insensitive",
			msg:        "Openat X: No Such File Or Directory",
			wantErr:    ErrFileNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "work_directory_unavailable_stays_internal",
			msg:  "work directory unavailable: open /srv/gameap: no such file or directory",
		},
		{
			name: "work_directory_permission_stays_internal",
			msg:  "work directory unavailable: open /srv/gameap: permission denied",
		},
		{
			name: "unrelated_daemon_text_stays_internal",
			msg:  "checksum mismatch: expected abc, got def",
		},
		{
			name: "empty_message_stays_internal",
			msg:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ACT
			err := daemonFileError("stat", tt.msg)

			// ASSERT
			if tt.wantErr == nil {
				assert.NoError(t, err, "message must not be classified")

				return
			}

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)

			var fileErr *FileError
			require.ErrorAs(t, err, &fileErr)
			assert.Equal(t, "stat", fileErr.Op)
			assert.Equal(t, tt.msg, fileErr.Detail, "raw daemon text must be kept for logs")
			assert.Equal(t, tt.wantStatus, fileErr.HTTPStatus())
			assert.Equal(t, tt.wantErr.Error(), err.Error(), "client-facing text must be the kind only")
			assert.NotContains(t, err.Error(), "servers/", "client-facing text must not carry node paths")
			assert.Equal(t, "stat: "+tt.msg, fileErr.Description())
		})
	}
}

func TestDaemonFileError_falsePositiveGuards(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		msg     string
		wantErr error
		notErr  error
	}{
		{
			name:    "does_not_exist_is_not_already_exists",
			msg:     "open x: file does not exist",
			wantErr: ErrFileNotFound,
			notErr:  ErrFileExists,
		},
		{
			name:    "is_not_a_directory_is_not_is_a_directory",
			msg:     `path "x" is not a directory`,
			wantErr: ErrNotDirectory,
			notErr:  ErrIsDirectory,
		},
		{
			name:    "directory_is_not_empty_is_not_not_directory",
			msg:     "unlinkat x: The directory is not empty.",
			wantErr: ErrDirectoryNotEmpty,
			notErr:  ErrNotDirectory,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := daemonFileError("op", tt.msg)

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.NotErrorIs(t, err, tt.notErr)
		})
	}
}

func TestClassifyFileError(t *testing.T) {
	t.Parallel()

	t.Run("nil_stays_nil", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, classifyFileError("file write", nil))
	})

	t.Run("recognised_text_becomes_FileError", func(t *testing.T) {
		t.Parallel()

		err := classifyFileError("file write", errors.New("openat servers/q2/x.cfg: permission denied"))

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPermissionDenied)
		assert.Equal(t, "permission denied", err.Error())

		var fileErr *FileError
		require.ErrorAs(t, err, &fileErr)
		assert.Equal(t, http.StatusForbidden, fileErr.HTTPStatus())
		assert.Equal(t, "file write", fileErr.Op)
	})

	t.Run("FileError_is_returned_unchanged", func(t *testing.T) {
		t.Parallel()

		orig := &FileError{
			Op:     "file read",
			Err:    ErrPermissionDenied,
			Detail: "openat servers/q2/x.cfg: permission denied",
		}

		err := classifyFileError("upload task", orig)

		assert.Same(t, orig, err, "an already classified error must keep its Op and Detail")
	})

	t.Run("wrapped_FileError_is_returned_unchanged", func(t *testing.T) {
		t.Parallel()

		orig := errors.WithMessage(&FileError{
			Op:     "file read",
			Err:    ErrPermissionDenied,
			Detail: "openat servers/q2/x.cfg: permission denied",
		}, "dispatched file read")

		err := classifyFileError("upload task", orig)

		assert.Same(t, orig, err, "wrapping must not be replaced by a fresh classification")

		var fileErr *FileError
		require.ErrorAs(t, err, &fileErr)
		assert.Equal(t, "file read", fileErr.Op)
		assert.Equal(t, "openat servers/q2/x.cfg: permission denied", fileErr.Detail)
	})

	t.Run("unrecognised_error_is_returned_unchanged", func(t *testing.T) {
		t.Parallel()

		orig := errors.New("node not connected")

		err := classifyFileError("file write", orig)

		assert.Same(t, orig, err)
	})

	t.Run("wrapped_FileError_keeps_status_and_description", func(t *testing.T) {
		t.Parallel()

		err := errors.WithMessage(
			classifyFileError("upload task", errors.New("write servers/q2/big.pak: no space left on device")),
			"upload task",
		)

		var statusErr interface{ HTTPStatus() int }
		require.ErrorAs(t, err, &statusErr)
		assert.Equal(t, http.StatusInsufficientStorage, statusErr.HTTPStatus())

		var descErr interface{ Description() string }
		require.ErrorAs(t, err, &descErr)
		assert.Equal(t, "upload task: write servers/q2/big.pak: no space left on device", descErr.Description())
		assert.Equal(t, "upload task: no space left on device", err.Error())
	})
}
