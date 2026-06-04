package filemanagerpath

import (
	"slices"
	"strings"

	"github.com/pkg/errors"
)

var (
	ErrPathContainsTraversal         = errors.New("path contains invalid directory traversal")
	ErrPathContainsNullByte          = errors.New("path contains a null byte")
	ErrPathEscapesBaseDirectory      = errors.New("path attempts to escape base directory")
	ErrFilenameEmpty                 = errors.New("filename is empty")
	ErrFilenameContainsTraversal     = errors.New("filename contains invalid directory traversal")
	ErrFilenameContainsPathSeparator = errors.New("filename contains path separators")
)

// ValidatePath rejects paths that could escape the server data directory.
//
// File-manager paths are addressed relative to the server data directory; a
// leading "/" denotes that root, not a filesystem-absolute path (the API
// joins the value under node.WorkPath/server.Dir). A Windows daemon or a
// Windows API host produces "\"-separated paths, so separators are normalized
// before the components are inspected. The check therefore:
//   - rejects NUL bytes;
//   - normalizes "\" to "/" so a Windows-style path is treated as one path and
//     every ".." component is visible to the traversal check (a backslash is a
//     legitimate separator, not an error);
//   - rejects any ".." path component, the only real escape vector once the
//     value is joined under the base;
//   - treats a sole "/" or "" as the server root.
//
// A literal substring like "ok..ok" is allowed (it is not a ".." component).
func ValidatePath(p string) error {
	if p == "" {
		return nil
	}

	if strings.ContainsRune(p, '\x00') {
		return ErrPathContainsNullByte
	}

	rel := strings.TrimPrefix(strings.ReplaceAll(p, "\\", "/"), "/")
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
