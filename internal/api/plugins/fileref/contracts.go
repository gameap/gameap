package fileref

import (
	"context"
	"io"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
)

// FileService is the node file surface needed to stream a file; satisfied by
// *daemon.FileService.
type FileService interface {
	GetFileInfo(ctx context.Context, node *domain.Node, path string) (*daemon.FileDetails, error)
	DownloadStream(ctx context.Context, node *domain.Node, filePath string) (io.ReadCloser, error)
	// DownloadStreamRange serves a byte range, which is what makes an
	// interrupted download resumable instead of starting over.
	DownloadStreamRange(
		ctx context.Context, node *domain.Node, filePath string, offset, length uint64,
	) (io.ReadCloser, error)
}

// PermissionChecker answers whether the plugin holds a grant; satisfied by
// *hostlibrary.RepositoryPermissionChecker, which denies plugin ID 0.
type PermissionChecker interface {
	Has(ctx context.Context, pluginID uint64, permission domain.PluginPermission) (bool, error)
}
