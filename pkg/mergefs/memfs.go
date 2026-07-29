package mergefs

import (
	"bytes"
	"io/fs"
	"path"
	"time"

	"github.com/pkg/errors"
)

// FromFiles builds a read-only in-memory fs.FS from a set of path→content
// entries. Paths must be valid fs paths (fs.ValidPath): slash-separated,
// unrooted, without "." or ".." elements. Ancestor directories are synthesised
// so the tree can be walked and listed. Opened files implement io.ReadSeeker,
// which the layered MergeFS relies on to keep files seekable for HTTP serving.
func FromFiles(files map[string][]byte) (fs.FS, error) {
	m := &memFS{
		files: make(map[string][]byte, len(files)),
		dirs:  map[string]map[string]fs.DirEntry{".": {}},
	}

	for name, content := range files {
		if !fs.ValidPath(name) || name == "." {
			return nil, errors.Errorf("invalid file path: %q", name)
		}

		m.files[name] = content
		m.link(path.Dir(name), memFileInfo{name: path.Base(name), size: int64(len(content))})

		for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
			m.link(path.Dir(dir), memFileInfo{name: path.Base(dir), dir: true})
		}
	}

	return m, nil
}

type memFS struct {
	files map[string][]byte
	dirs  map[string]map[string]fs.DirEntry
}

// link registers child as an entry of parent, creating parent's entry set on
// first use. A name reached more than once (a directory holding several files)
// keeps its first registration.
func (m *memFS) link(parent string, child memFileInfo) {
	entries, ok := m.dirs[parent]
	if !ok {
		entries = make(map[string]fs.DirEntry)
		m.dirs[parent] = entries
	}

	if _, exists := entries[child.name]; !exists {
		entries[child.name] = child
	}
}

func (m *memFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	if content, ok := m.files[name]; ok {
		return &openMemFile{
			info:   memFileInfo{name: path.Base(name), size: int64(len(content))},
			Reader: bytes.NewReader(content),
		}, nil
	}

	if entries, ok := m.dirs[name]; ok {
		return &dirFile{
			info:    memFileInfo{name: path.Base(name), dir: true},
			entries: sortedEntries(entries),
		}, nil
	}

	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (m *memFS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}

	if content, ok := m.files[name]; ok {
		return memFileInfo{name: path.Base(name), size: int64(len(content))}, nil
	}

	if _, ok := m.dirs[name]; ok {
		return memFileInfo{name: path.Base(name), dir: true}, nil
	}

	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

func (m *memFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}

	entries, ok := m.dirs[name]
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}

	return sortedEntries(entries), nil
}

func (m *memFS) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	content, ok := m.files[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	result := make([]byte, len(content))
	copy(result, content)

	return result, nil
}

// openMemFile is an open regular file. The embedded *bytes.Reader supplies Read
// and Seek, so the file satisfies io.ReadSeeker.
type openMemFile struct {
	*bytes.Reader

	info memFileInfo
}

func (f *openMemFile) Stat() (fs.FileInfo, error) { return f.info, nil }

func (f *openMemFile) Close() error { return nil }

// memFileInfo implements both fs.FileInfo and fs.DirEntry for in-memory entries
// and the synthetic directories of the layered filesystem.
type memFileInfo struct {
	name string
	size int64
	dir  bool
}

func (i memFileInfo) Name() string { return i.name }

func (i memFileInfo) Size() int64 { return i.size }

func (i memFileInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}

	return 0o444
}

func (i memFileInfo) ModTime() time.Time { return time.Time{} }

func (i memFileInfo) IsDir() bool { return i.dir }

func (i memFileInfo) Sys() any { return nil }

func (i memFileInfo) Info() (fs.FileInfo, error) { return i, nil }

func (i memFileInfo) Type() fs.FileMode { return i.Mode().Type() }
