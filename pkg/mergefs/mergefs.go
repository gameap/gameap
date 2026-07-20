// Package mergefs provides a read-only, layered fs.FS. Several filesystems are
// stacked; a lookup resolves to the first layer that contains the name, so
// earlier layers shadow later ones. Directory listings are the union of every
// layer that holds the directory, with earlier layers winning on name
// collisions.
//
// Files are returned from the underlying layer unchanged, so a layer whose
// files implement io.ReadSeeker (embed.FS, or the in-memory FS from FromFiles)
// keeps that capability — required by http.ServeContent and http.FileServer.
package mergefs

import (
	"io"
	"io/fs"
	"path"
	"sort"

	"github.com/pkg/errors"
)

var errIsDir = errors.New("is a directory")

// MergeFS is an ordered stack of filesystems. Build one with New or NewDynamic.
type MergeFS struct {
	layers func() []fs.FS
}

// New returns a MergeFS over a fixed, ordered set of layers. Earlier layers
// take precedence over later ones.
func New(layers ...fs.FS) *MergeFS {
	snapshot := make([]fs.FS, len(layers))
	copy(snapshot, layers)

	return &MergeFS{
		layers: func() []fs.FS { return snapshot },
	}
}

// NewDynamic returns a MergeFS whose layers are resolved by provider on every
// operation, so a changing set of layers (e.g. plugins loaded at runtime) is
// reflected without rebuilding. Earlier layers take precedence.
func NewDynamic(provider func() []fs.FS) *MergeFS {
	return &MergeFS{layers: provider}
}

// Open resolves name against the layers in order. A regular file is returned
// from the first layer that holds it, unchanged, so io.ReadSeeker is preserved.
// A directory present in any layer yields a merged directory whose entries are
// the union of every layer that holds it. The first layer to hold name decides
// its kind: a file in an earlier layer shadows a directory in a later one and
// vice versa.
func (m *MergeFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	var (
		dirLayers []fs.FS
		isDir     bool
	)

	for _, layer := range m.layers() {
		if layer == nil {
			continue
		}

		f, err := layer.Open(name)
		if err != nil {
			continue
		}

		info, err := f.Stat()
		if err != nil {
			_ = f.Close()

			continue
		}

		if !info.IsDir() {
			if isDir {
				_ = f.Close()

				continue
			}

			return f, nil
		}

		_ = f.Close()
		isDir = true
		dirLayers = append(dirLayers, layer)
	}

	if isDir {
		return newMergedDir(name, dirLayers), nil
	}

	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// Stat reports the info of name from the first layer that holds it.
func (m *MergeFS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}

	for _, layer := range m.layers() {
		if layer == nil {
			continue
		}

		info, err := fs.Stat(layer, name)
		if err != nil {
			continue
		}

		return info, nil
	}

	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

// ReadFile returns the content of the first-layer file named name. It delegates
// to Open so shadowing semantics are identical; reading a directory is an error.
func (m *MergeFS) ReadFile(name string) ([]byte, error) {
	f, err := m.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	return io.ReadAll(f)
}

// ReadDir returns the union of name's entries across every layer that holds it,
// deduplicated by name (earlier layers win) and sorted.
func (m *MergeFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}

	merged := make(map[string]fs.DirEntry)
	found := false

	for _, layer := range m.layers() {
		if layer == nil {
			continue
		}

		entries, err := fs.ReadDir(layer, name)
		if err != nil {
			continue
		}

		found = true

		for _, entry := range entries {
			if _, exists := merged[entry.Name()]; !exists {
				merged[entry.Name()] = entry
			}
		}
	}

	if !found {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}

	return sortedEntries(merged), nil
}

func newMergedDir(name string, layers []fs.FS) *dirFile {
	merged := make(map[string]fs.DirEntry)

	for _, layer := range layers {
		entries, err := fs.ReadDir(layer, name)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if _, exists := merged[entry.Name()]; !exists {
				merged[entry.Name()] = entry
			}
		}
	}

	return &dirFile{
		info:    memFileInfo{name: path.Base(name), dir: true},
		entries: sortedEntries(merged),
	}
}

func sortedEntries(entries map[string]fs.DirEntry) []fs.DirEntry {
	result := make([]fs.DirEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})

	return result
}

// dirFile is a read-only fs.ReadDirFile backed by a precomputed, sorted entry
// list. It is shared by the union directories MergeFS synthesises and the
// directories of the in-memory FS from FromFiles.
type dirFile struct {
	info    fs.FileInfo
	entries []fs.DirEntry
	offset  int
}

func (d *dirFile) Stat() (fs.FileInfo, error) { return d.info, nil }

func (d *dirFile) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.info.Name(), Err: errIsDir}
}

func (d *dirFile) Close() error { return nil }

func (d *dirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		remaining := d.entries[d.offset:]
		d.offset = len(d.entries)

		return remaining, nil
	}

	if d.offset >= len(d.entries) {
		return nil, io.EOF
	}

	end := min(d.offset+n, len(d.entries))
	remaining := d.entries[d.offset:end]
	d.offset = end

	return remaining, nil
}
