package files

import "context"

type MockFileManager struct {
	ReadFunc   func(ctx context.Context, path string) ([]byte, error)
	WriteFunc  func(ctx context.Context, path string, data []byte) error
	DeleteFunc func(ctx context.Context, path string) error
	ExistsFunc func(ctx context.Context, path string) bool
	ListFunc   func(ctx context.Context, dir string) ([]string, error)
}

func (m *MockFileManager) Read(ctx context.Context, path string) ([]byte, error) {
	if m.ReadFunc != nil {
		return m.ReadFunc(ctx, path)
	}

	return nil, nil
}

func (m *MockFileManager) Write(ctx context.Context, path string, data []byte) error {
	if m.WriteFunc != nil {
		return m.WriteFunc(ctx, path, data)
	}

	return nil
}

func (m *MockFileManager) Delete(ctx context.Context, path string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, path)
	}

	return nil
}

func (m *MockFileManager) Exists(ctx context.Context, path string) bool {
	if m.ExistsFunc != nil {
		return m.ExistsFunc(ctx, path)
	}

	return false
}

func (m *MockFileManager) List(ctx context.Context, dir string) ([]string, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, dir)
	}

	return nil, nil
}
