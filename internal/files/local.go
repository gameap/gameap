package files

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const (
	defaultLocalFilePerm = 0644
	defaultLocalDirPerm  = 0755
)

type LocalFileManager struct {
	root *os.Root
}

func NewLocalFileManager(basePath string) *LocalFileManager {
	root, err := os.OpenRoot(basePath)
	if err != nil {
		panic(fmt.Sprintf("failed to open root directory: %v", err))
	}

	return &LocalFileManager{
		root: root,
	}
}

func (fm *LocalFileManager) Read(_ context.Context, path string) ([]byte, error) {
	b, err := fm.root.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read file")
	}

	return b, nil
}

func (fm *LocalFileManager) Write(ctx context.Context, path string, data []byte) error {
	if !fm.Exists(ctx, path) {
		err := fm.mkdirAll(filepath.Dir(path))
		if err != nil {
			return errors.Wrapf(err, "failed to create directories: %s", filepath.Dir(path))
		}
	}

	err := fm.root.WriteFile(path, data, defaultLocalFilePerm)
	if err != nil {
		return errors.Wrap(err, "failed to write file")
	}

	return nil
}

func (fm *LocalFileManager) mkdirAll(path string) error {
	if path == "" || path == "." {
		return nil
	}

	_, err := fm.root.Stat(path)
	if err == nil {
		return nil
	}

	parent := filepath.Dir(path)
	if parent != path && parent != "." {
		if err := fm.mkdirAll(parent); err != nil {
			return errors.Wrapf(err, "failed to create directory: %s", parent)
		}
	}

	err = fm.root.Mkdir(path, defaultLocalDirPerm)
	if err != nil && !os.IsExist(err) {
		return errors.Wrapf(err, "failed to create directory: %s", path)
	}

	return nil
}

func (fm *LocalFileManager) Delete(_ context.Context, path string) error {
	err := fm.root.Remove(path)
	if err != nil {
		return errors.Wrap(err, "failed to delete file")
	}

	return nil
}

func (fm *LocalFileManager) Exists(_ context.Context, path string) bool {
	_, err := fm.root.Stat(path)

	return err == nil
}

func (fm *LocalFileManager) List(_ context.Context, dir string) ([]string, error) {
	dirFile, err := fm.root.Open(dir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open directory")
	}
	defer func(dirFile *os.File) {
		err := dirFile.Close()
		if err != nil {
			slog.Warn(fmt.Sprintf("failed to close directory file: %v", err))
		}
	}(dirFile)

	entries, err := dirFile.ReadDir(-1)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read directory")
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		files = append(files, entry.Name())
	}

	return files, nil
}
