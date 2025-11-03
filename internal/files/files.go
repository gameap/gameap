package files

import "context"

type FileManager interface {
	Read(ctx context.Context, path string) ([]byte, error)
	Write(ctx context.Context, path string, data []byte) error
	Delete(ctx context.Context, path string) error
	Exists(ctx context.Context, path string) bool
	List(ctx context.Context, dir string) ([]string, error)
}
