// Security tests for the centralized file-manager path validation.
//
// OWASP API Security Top 10:2023 — API1:2023 Broken Object Level Authorization
// (path traversal lets a user reach files outside their server sandbox, i.e.
// objects they are not authorized for). See security review finding #2.
package filemanagerpath_test

import (
	"testing"

	"github.com/gameap/gameap/internal/api/filemanager/filemanagerpath"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidatePath — OWASP API1:2023 Broken Object Level Authorization.
func TestValidatePath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantError string
	}{
		{
			name: "valid_relative_path",
			path: "configs/server.cfg",
		},
		{
			name: "valid_single_directory",
			path: "configs",
		},
		{
			name: "valid_root_dot",
			path: ".",
		},
		{
			name: "valid_empty_path",
			path: "",
		},
		{
			name: "valid_root_slash",
			path: "/",
		},
		{
			name: "valid_root_relative_leading_slash",
			path: "/configs/server.cfg",
		},
		{
			name: "valid_dotfile_in_filename",
			path: "configs/.hidden",
		},
		{
			name: "valid_double_dots_inside_segment",
			path: "ok..ok/version2..1",
		},
		{
			name:      "invalid_directory_traversal",
			path:      "../../../etc/passwd",
			wantError: "path contains invalid directory traversal",
		},
		{
			name:      "invalid_path_with_double_dots_in_middle",
			path:      "configs/../../etc",
			wantError: "path contains invalid directory traversal",
		},
		{
			name:      "invalid_double_dot_only",
			path:      "..",
			wantError: "path contains invalid directory traversal",
		},
		{
			name:      "invalid_leading_slash_traversal",
			path:      "/../etc/passwd",
			wantError: "path contains invalid directory traversal",
		},
		{
			name:      "invalid_trailing_traversal",
			path:      "configs/..",
			wantError: "path contains invalid directory traversal",
		},
		{
			name:      "invalid_backslash_windows_traversal",
			path:      "configs\\..\\..\\windows",
			wantError: "path contains a backslash",
		},
		{
			name:      "invalid_plain_backslash",
			path:      "dir\\file.txt",
			wantError: "path contains a backslash",
		},
		{
			name:      "invalid_null_byte",
			path:      "configs/server\x00.cfg",
			wantError: "path contains a null byte",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := filemanagerpath.ValidatePath(tt.path)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}
			assert.NoError(t, err)
		})
	}
}

// TestValidateFilename — OWASP API1:2023 Broken Object Level Authorization.
func TestValidateFilename(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		wantError string
	}{
		{
			name:     "valid_filename",
			filename: "test.txt",
		},
		{
			name:     "valid_filename_with_dots",
			filename: "server.properties.backup",
		},
		{
			name:     "valid_dotfile",
			filename: ".env",
		},
		{
			name:      "empty_filename",
			filename:  "",
			wantError: "filename is empty",
		},
		{
			name:      "filename_with_directory_traversal",
			filename:  "../test.txt",
			wantError: "filename contains invalid directory traversal",
		},
		{
			name:      "filename_with_just_double_dot",
			filename:  "..",
			wantError: "filename contains invalid directory traversal",
		},
		{
			name:      "filename_with_forward_slash",
			filename:  "dir/test.txt",
			wantError: "filename contains path separators",
		},
		{
			name:      "filename_with_backslash",
			filename:  "dir\\test.txt",
			wantError: "filename contains path separators",
		},
		{
			name:      "filename_with_only_forward_slash",
			filename:  "/",
			wantError: "filename contains path separators",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := filemanagerpath.ValidateFilename(tt.filename)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}
			assert.NoError(t, err)
		})
	}
}
