package filemanagerpath

import (
	"slices"
	"strings"

	"github.com/pkg/errors"
)

var (
	ErrPathContainsTraversal         = errors.New("path contains invalid directory traversal")
	ErrPathContainsNullByte          = errors.New("path contains a null byte")
	ErrPathContainsBackslash         = errors.New("path contains a backslash")
	ErrPathEscapesBaseDirectory      = errors.New("path attempts to escape base directory")
	ErrFilenameEmpty                 = errors.New("filename is empty")
	ErrFilenameContainsTraversal     = errors.New("filename contains invalid directory traversal")
	ErrFilenameContainsPathSeparator = errors.New("filename contains path separators")
)

// ValidatePath rejects paths that could escape the server data directory.
//
// File-manager paths are addressed relative to the server data directory; a
// leading "/" denotes that root, not a filesystem-absolute path (the API
// joins the value under node.WorkPath/server.Dir). The check therefore:
//   - rejects NUL bytes and backslashes (the latter is a path separator on a
//     Windows daemon and would otherwise bypass POSIX-only normalization);
//   - rejects any ".." path component (before normalization), which is the
//     only real escape vector once the value is joined under the base;
//   - treats a sole "/" or "" as the server root.
//
// A literal substring like "ok..ok" is allowed (it is not a ".." component),
// fixing the previous false-positive rejection.
func ValidatePath(p string) error {
	if p == "" {
		return nil
	}

	if strings.ContainsRune(p, '\x00') {
		return ErrPathContainsNullByte
	}

	if strings.ContainsRune(p, '\\') {
		return ErrPathContainsBackslash
	}

	rel := strings.TrimPrefix(p, "/")
	if rel == "" {
		return nil
	}

	if slices.Contains(strings.Split(rel, "/"), "..") {
		return ErrPathContainsTraversal
	}

	return nil
}

func ValidateFilename(filename string) error {
	if filename == "" {
		return ErrFilenameEmpty
	}

	if strings.Contains(filename, "..") {
		return ErrFilenameContainsTraversal
	}

	if strings.ContainsAny(filename, "/\\") {
		return ErrFilenameContainsPathSeparator
	}

	return nil
}
