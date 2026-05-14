package upload

import (
	"context"
	"io"
	"time"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
)

type Storage interface {
	Read(ctx context.Context, path string) ([]byte, error)
	Write(ctx context.Context, path string, data []byte) error
	WriteStream(ctx context.Context, path string, data io.Reader) error
	ReadStream(ctx context.Context, path string) (io.ReadCloser, error)
	Exists(ctx context.Context, path string) bool
	List(ctx context.Context, dir string) ([]string, error)
	Delete(ctx context.Context, path string) error
	DeleteByPrefix(ctx context.Context, prefix string) error
}

type DaemonUploader interface {
	UploadStreamPrepared(
		ctx context.Context,
		node *domain.Node,
		relPath string,
		transferID string,
		checksum string,
		totalSize uint64,
		owner daemon.OwnerOptions,
	) error
}

type NodeFinder interface {
	FindByID(ctx context.Context, nodeID uint) (*domain.Node, error)
}

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func RealClock() Clock { return realClock{} }
