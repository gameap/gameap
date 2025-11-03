package files

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// InMemoryFileManager is an in-memory implementation of FileManager for testing purposes.
type InMemoryFileManager struct {
	mu    sync.RWMutex
	files map[string][]byte
}

// NewInMemoryFileManager creates a new InMemoryFileManager instance.
func NewInMemoryFileManager() *InMemoryFileManager {
	return &InMemoryFileManager{
		files: make(map[string][]byte),
	}
}

func (fm *InMemoryFileManager) Read(_ context.Context, path string) ([]byte, error) {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	data, exists := fm.files[path]
	if !exists {
		return nil, fmt.Errorf("file not found: %s", path) //nolint:err113
	}

	return data, nil
}

func (fm *InMemoryFileManager) Write(_ context.Context, path string, data []byte) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	fm.files[path] = data

	return nil
}

func (fm *InMemoryFileManager) Delete(_ context.Context, path string) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	delete(fm.files, path)

	return nil
}

func (fm *InMemoryFileManager) Exists(_ context.Context, path string) bool {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	_, exists := fm.files[path]

	return exists
}

func (fm *InMemoryFileManager) List(_ context.Context, dir string) ([]string, error) {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	var result []string
	for path := range fm.files {
		if strings.HasPrefix(path, dir) {
			result = append(result, path)
		}
	}

	return result, nil
}

var _ FileManager = (*InMemoryFileManager)(nil)
