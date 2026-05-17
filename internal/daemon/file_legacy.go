package daemon

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"math"
	"net"
	"os"
	"path"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/daemon/binnapi"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"
)

const (
	filesRetryCount = 2
	filesRetryDelay = 10 * time.Millisecond

	// legacyUploadIdleTimeout caps the gap between successful TCP writes during
	// io.Copy. Slow networks (Tailscale, VPN, NAT) can deliver bytes at very
	// low throughput legitimately; we only fail when the stream truly stalls.
	legacyUploadIdleTimeout = 60 * time.Second
	// legacyUploadHandshakeDeadline covers Connect → WriteMessage → first
	// ReadMessage(ReadyToTransfer). Generous to absorb TLS+Login under load.
	legacyUploadHandshakeDeadline = 60 * time.Second
	// legacyUploadFinalReadDeadline covers the final ReadMessage(StatusOK)
	// after io.Copy completes. Daemon also has to copy temp→final on disk.
	legacyUploadFinalReadDeadline = 60 * time.Second
	// legacyUploadCopyBufferSize bounds how many bytes a single conn.Write
	// pushes at once. Smaller chunks → shorter per-Write block under TCP
	// backpressure → idle-timeout refresh actually fires between chunks.
	legacyUploadCopyBufferSize = 32 * 1024
)

type FileBINNService struct {
	configMaker *configMaker

	mu    sync.RWMutex
	pools map[uint]*Pool
}

func NewFileBINNService(
	certRepo repositories.ClientCertificateRepository,
	fileManager files.FileManager,
	opts ...LegacyOption,
) *FileBINNService {
	cm := newConfigMaker(certRepo, fileManager)
	for _, opt := range opts {
		opt(cm)
	}

	return &FileBINNService{
		configMaker: cm,
		pools:       make(map[uint]*Pool),
	}
}

// ReadDir reads the contents of a directory.
func (s *FileBINNService) ReadDir(
	ctx context.Context,
	node *domain.Node,
	directory string,
) ([]*FileInfo, error) {
	cfg, err := s.configMaker.MakeWithMode(ctx, node, binnapi.ModeFiles)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to make config")
	}

	pool, err := s.getPool(node.ID, cfg)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get pool")
	}

	var resp binnapi.BaseResponseMessage

	err = Retry(filesRetryCount, filesRetryDelay, func() error {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return errors.WithMessage(err, "failed to acquire connection from pool")
		}
		defer func() {
			err = conn.Close()
			if err != nil {
				slog.Warn("failed to close connection", "error", err)
			}
		}()

		err = binnapi.WriteMessage(conn, &binnapi.ReadDirRequestMessage{
			Directory:   directory,
			DetailsMode: false,
		})
		if err != nil {
			return errors.WithMessage(err, "failed to write read dir request")
		}

		err = binnapi.ReadMessage(conn, &resp)
		if err != nil {
			return errors.WithMessage(err, "failed to read read dir response")
		}

		return nil
	})
	if err != nil {
		return nil, errors.WithMessagef(
			err,
			"failed to read directory after %d attempts",
			filesRetryCount,
		)
	}

	if resp.Code != binnapi.StatusCodeOK {
		return nil, errors.Errorf("read dir failed with status code %d: %s", resp.Code, resp.Info)
	}

	fileList, ok := resp.Data.([]any)
	if !ok {
		return nil, errors.New("invalid response data format")
	}

	var resultList = make([]*FileInfo, 0, len(fileList))
	for _, item := range fileList {
		fileData, ok := item.([]any)
		if !ok {
			return nil, errors.New("invalid file info format")
		}

		binnapiFileInfo, err := binnapi.CreateFileInfoResponseMessage(fileData)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse file info")
		}

		resultList = append(resultList, &FileInfo{
			Name:         binnapiFileInfo.Name,
			Size:         binnapiFileInfo.Size,
			TimeModified: binnapiFileInfo.TimeModified,
			Type:         FileType(binnapiFileInfo.Type),
			Perm:         binnapiFileInfo.Perm,
		})
	}

	return resultList, nil
}

// ReadDirRecursive walks the directory tree rooted at directory and returns a
// flat list of FileInfo entries with FileInfo.Path filled relative to node.WorkPath.
func (s *FileBINNService) ReadDirRecursive(
	ctx context.Context,
	node *domain.Node,
	directory string,
) ([]*FileInfo, error) {
	return s.readDirRecursive(ctx, node, directory)
}

func (s *FileBINNService) readDirRecursive(
	ctx context.Context,
	node *domain.Node,
	directory string,
) ([]*FileInfo, error) {
	entries, err := s.ReadDir(ctx, node, directory)
	if err != nil {
		return nil, err
	}

	result := make([]*FileInfo, 0, len(entries))
	for _, entry := range entries {
		childAbs := path.Join(directory, entry.Name)
		entry.Path = stripWorkPath(node.WorkPath, childAbs)
		result = append(result, entry)

		if entry.Type != FileTypeDir {
			continue
		}

		children, err := s.readDirRecursive(ctx, node, childAbs)
		if err != nil {
			return nil, errors.WithMessagef(err, "recursive read dir %q", childAbs)
		}

		result = append(result, children...)
	}

	return result, nil
}

// MkDir creates a directory.
func (s *FileBINNService) MkDir(ctx context.Context, node *domain.Node, directory string) error {
	cfg, err := s.configMaker.MakeWithMode(ctx, node, binnapi.ModeFiles)
	if err != nil {
		return errors.WithMessage(err, "failed to make config")
	}

	pool, err := s.getPool(node.ID, cfg)
	if err != nil {
		return errors.WithMessage(err, "failed to get pool")
	}

	var resp binnapi.BaseResponseMessage

	err = Retry(filesRetryCount, filesRetryDelay, func() error {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return errors.WithMessage(err, "failed to acquire connection from pool")
		}
		defer func() {
			err = conn.Close()
			if err != nil {
				slog.Warn("failed to close connection", "error", err)
			}
		}()

		err = binnapi.WriteMessage(conn, &binnapi.MkDirRequestMessage{
			Directory: directory,
		})
		if err != nil {
			return errors.WithMessage(err, "failed to write mkdir request")
		}

		err = binnapi.ReadMessage(conn, &resp)
		if err != nil {
			return errors.WithMessage(err, "failed to read mkdir response")
		}

		return nil
	})
	if err != nil {
		return errors.WithMessagef(
			err,
			"failed to create directory after %d attempts",
			filesRetryCount,
		)
	}

	if resp.Code != binnapi.StatusCodeOK {
		return errors.Errorf("mkdir failed with status code %d: %s", resp.Code, resp.Info)
	}

	return nil
}

func (s *FileBINNService) Copy(ctx context.Context, node *domain.Node, source, destination string) error {
	return s.move(ctx, node, source, destination, true)
}

func (s *FileBINNService) Move(ctx context.Context, node *domain.Node, source, destination string) error {
	return s.move(ctx, node, source, destination, false)
}

// Move moves or copies a file.
func (s *FileBINNService) move(ctx context.Context, node *domain.Node, source, destination string, cp bool) error {
	cfg, err := s.configMaker.MakeWithMode(ctx, node, binnapi.ModeFiles)
	if err != nil {
		return errors.WithMessage(err, "failed to make config")
	}

	pool, err := s.getPool(node.ID, cfg)
	if err != nil {
		return errors.WithMessage(err, "failed to get pool")
	}

	var resp binnapi.BaseResponseMessage

	err = Retry(filesRetryCount, filesRetryDelay, func() error {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return errors.WithMessage(err, "failed to acquire connection from pool")
		}
		defer func() {
			err = conn.Close()
			if err != nil {
				slog.Warn("failed to close connection", "error", err)
			}
		}()

		err = binnapi.WriteMessage(conn, &binnapi.MoveRequestMessage{
			Source:      source,
			Destination: destination,
			Copy:        cp,
		})
		if err != nil {
			return errors.WithMessage(err, "failed to write move request")
		}

		err = binnapi.ReadMessage(conn, &resp)
		if err != nil {
			return errors.WithMessage(err, "failed to read move response")
		}

		return nil
	})
	if err != nil {
		return errors.WithMessagef(
			err,
			"failed to move/copy file after %d attempts",
			filesRetryCount,
		)
	}

	if resp.Code != binnapi.StatusCodeOK {
		return errors.Errorf("move failed with status code %d: %s", resp.Code, resp.Info)
	}

	return nil
}

// Download downloads a file from the daemon.
func (s *FileBINNService) Download(ctx context.Context, node *domain.Node, filePath string) ([]byte, error) {
	cfg, err := s.configMaker.MakeWithMode(ctx, node, binnapi.ModeFiles)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to make config")
	}

	pool, err := s.getPool(node.ID, cfg)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get pool")
	}

	var resp binnapi.BaseResponseMessage
	var file []byte

	err = Retry(filesRetryCount, filesRetryDelay, func() error {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return errors.WithMessage(err, "failed to acquire connection from pool")
		}
		defer func() {
			err = conn.Close()
			if err != nil {
				slog.Warn("failed to close connection", "error", err)
			}
		}()

		err = binnapi.WriteMessage(conn, &binnapi.DownloadRequestMessage{
			FilePath: filePath,
		})
		if err != nil {
			return errors.WithMessage(err, "failed to write download request")
		}

		err = binnapi.ReadMessage(conn, &resp)
		if err != nil {
			return errors.WithMessage(err, "failed to read download response")
		}

		if resp.Code != binnapi.StatusCodeReadyToTransfer {
			return errors.Errorf("download failed with status code %d: %s", resp.Code, resp.Info)
		}

		fileSize, err := binnapi.CreateFileSize(resp.Data)
		if err != nil {
			return errors.WithMessage(err, "failed to get file size")
		}

		if fileSize == 0 {
			file = []byte{}

			return nil
		}

		file = make([]byte, fileSize)
		_, err = io.ReadFull(conn, file)
		if err != nil {
			return errors.WithMessage(err, "failed to read file content")
		}

		return nil
	})
	if err != nil {
		return nil, errors.WithMessagef(
			err,
			"failed to download file after %d attempts",
			filesRetryCount,
		)
	}

	return file, nil
}

// DownloadStream downloads a file from the daemon as a stream.
// The caller is responsible for closing the returned ReadCloser.
func (s *FileBINNService) DownloadStream(
	ctx context.Context,
	node *domain.Node,
	filePath string,
) (io.ReadCloser, error) {
	cfg, err := s.configMaker.MakeWithMode(ctx, node, binnapi.ModeFiles)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to make config")
	}

	pool, err := s.getPool(node.ID, cfg)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get pool")
	}

	var conn net.Conn
	var resp binnapi.BaseResponseMessage
	var fileSize uint64

	err = Retry(filesRetryCount, filesRetryDelay, func() error {
		var err error
		conn, err = pool.Acquire(ctx)
		if err != nil {
			return errors.WithMessage(err, "failed to acquire connection from pool")
		}

		err = binnapi.WriteMessage(conn, &binnapi.DownloadRequestMessage{
			FilePath: filePath,
		})
		if err != nil {
			connCloseErr := conn.Close()
			if connCloseErr != nil {
				slog.Warn("failed to close connection", "error", connCloseErr)
			}

			return errors.WithMessage(err, "failed to write download request")
		}

		err = binnapi.ReadMessage(conn, &resp)
		if err != nil {
			connCloseErr := conn.Close()
			if connCloseErr != nil {
				slog.Warn("failed to close connection", "error", connCloseErr)
			}

			return errors.WithMessage(err, "failed to read download response")
		}

		if resp.Code != binnapi.StatusCodeReadyToTransfer {
			connCloseErr := conn.Close()
			if connCloseErr != nil {
				slog.Warn("failed to close connection", "error", connCloseErr)
			}

			return errors.Errorf("download failed with status code %d: %s", resp.Code, resp.Info)
		}

		fileSizeValue, err := binnapi.CreateFileSize(resp.Data)
		if err != nil {
			connCloseErr := conn.Close()
			if connCloseErr != nil {
				slog.Warn("failed to close connection", "error", connCloseErr)
			}

			return errors.WithMessage(err, "failed to get file size")
		}
		fileSize = uint64(fileSizeValue)

		return nil
	})
	if err != nil {
		return nil, errors.WithMessagef(
			err,
			"failed to initialize download stream after %d attempts",
			filesRetryCount,
		)
	}

	if fileSize > uint64(math.MaxInt64) {
		connCloseErr := conn.Close()
		if connCloseErr != nil {
			slog.Warn("failed to close connection", "error", connCloseErr)
		}

		return nil, errors.New("file size exceeds maximum supported size")
	}

	return &fileStreamReader{
		reader: io.LimitReader(conn, int64(fileSize)),
		closer: conn,
	}, nil
}

// fileStreamReader wraps an io.Reader and io.Closer to provide io.ReadCloser functionality.
type fileStreamReader struct {
	reader io.Reader
	closer io.Closer
}

func (f *fileStreamReader) Read(p []byte) (n int, err error) {
	return f.reader.Read(p)
}

func (f *fileStreamReader) Close() error {
	return f.closer.Close()
}

// Upload uploads a file to the daemon.
func (s *FileBINNService) Upload(
	ctx context.Context,
	node *domain.Node,
	filePath string,
	content []byte,
	perms os.FileMode,
) error {
	return s.UploadStream(
		ctx,
		node,
		filePath,
		bytes.NewReader(content),
		uint64(len(content)),
		perms,
	)
}

//nolint:funlen // verbose debug logging until upload bug is diagnosed
func (s *FileBINNService) UploadStream(
	ctx context.Context,
	node *domain.Node,
	filePath string,
	r io.Reader,
	size uint64,
	perms os.FileMode,
) error {
	slog.Debug("legacy upload: start",
		"node_id", node.ID, "file_path", filePath, "size", size, "perms", perms)

	cfg, err := s.configMaker.MakeWithMode(ctx, node, binnapi.ModeFiles)
	if err != nil {
		slog.Debug("legacy upload: make config failed", "node_id", node.ID, "error", err)

		return errors.WithMessage(err, "failed to make config")
	}

	slog.Debug("legacy upload: config ready",
		"node_id", node.ID, "host", cfg.Host, "port", cfg.Port,
		"idle_timeout", legacyUploadIdleTimeout)

	var resp binnapi.BaseResponseMessage
	attempt := 0

	err = Retry(filesRetryCount, filesRetryDelay, func() error {
		attempt++
		attemptStart := time.Now()
		slog.Debug("legacy upload: attempt start",
			"node_id", node.ID, "attempt", attempt, "size", size)

		if seeker, ok := r.(io.Seeker); ok {
			if _, seekErr := seeker.Seek(0, io.SeekStart); seekErr != nil {
				slog.Debug("legacy upload: reader seek failed",
					"node_id", node.ID, "attempt", attempt, "error", seekErr)

				return errors.Wrap(seekErr, "rewind upload reader for retry")
			}
		} else if attempt > 1 {
			slog.Debug("legacy upload: reader is not seekable, retry will be empty",
				"node_id", node.ID, "attempt", attempt)
		}

		connectStart := time.Now()
		conn, err := Connect(ctx, cfg)
		if err != nil {
			slog.Debug("legacy upload: connect failed",
				"node_id", node.ID, "attempt", attempt,
				"connect_duration", time.Since(connectStart), "error", err)

			return errors.Wrap(err, "open upload connection")
		}
		slog.Debug("legacy upload: connected",
			"node_id", node.ID, "attempt", attempt,
			"connect_duration", time.Since(connectStart),
			"local_addr", conn.LocalAddr(), "remote_addr", conn.RemoteAddr())
		defer func() {
			if closeErr := conn.Close(); closeErr != nil {
				slog.Warn("failed to close upload connection",
					"node_id", node.ID, "attempt", attempt, "error", closeErr)
			}
		}()

		if dErr := conn.SetDeadline(time.Now().Add(legacyUploadHandshakeDeadline)); dErr != nil {
			return errors.Wrap(dErr, "set handshake deadline")
		}

		writeReqStart := time.Now()
		err = binnapi.WriteMessage(conn, &binnapi.UploadRequestMessage{
			FilePath: filePath,
			FileSize: size,
			MakeDirs: true,
			Perms:    perms,
		})
		if err != nil {
			slog.Debug("legacy upload: write request failed",
				"node_id", node.ID, "attempt", attempt,
				"duration", time.Since(writeReqStart), "error", err)

			return errors.WithMessage(err, "failed to write upload request")
		}
		slog.Debug("legacy upload: request sent",
			"node_id", node.ID, "attempt", attempt,
			"duration", time.Since(writeReqStart))

		readReadyStart := time.Now()
		err = binnapi.ReadMessage(conn, &resp)
		if err != nil {
			slog.Debug("legacy upload: read ready response failed",
				"node_id", node.ID, "attempt", attempt,
				"duration", time.Since(readReadyStart), "error", err)

			return errors.WithMessage(err, "failed to read upload response")
		}
		slog.Debug("legacy upload: ready response received",
			"node_id", node.ID, "attempt", attempt,
			"duration", time.Since(readReadyStart),
			"status_code", resp.Code, "info", resp.Info)

		if resp.Code != binnapi.StatusCodeReadyToTransfer {
			return errors.Errorf("upload failed with status code %d: %s", resp.Code, resp.Info)
		}

		copyStart := time.Now()
		// Wrap reader to hide WriterTo. bytes.Reader implements io.WriterTo,
		// which io.Copy would call as a single Write(allBytes) — that single
		// write blocks past the idle-timeout on slow links (TCP backpressure
		// drains MBs at ~17 KB/s in user environment). Standard Read/Write
		// loop with 32 KB chunks gives us ~2 s per Write maximum, so
		// idle-timeout refresh actually fires per chunk.
		copied, copyErr := io.CopyBuffer(
			&idleDeadlineWriter{conn: conn, idleTimeout: legacyUploadIdleTimeout},
			struct{ io.Reader }{r},
			make([]byte, legacyUploadCopyBufferSize),
		)
		if copyErr != nil {
			slog.Debug("legacy upload: stream copy failed",
				"node_id", node.ID, "attempt", attempt,
				"copied", copied, "expected", size,
				"duration", time.Since(copyStart), "error", copyErr)

			return errors.WithMessage(copyErr, "failed to stream file content")
		}
		slog.Debug("legacy upload: stream copied",
			"node_id", node.ID, "attempt", attempt,
			"copied", copied, "expected", size,
			"duration", time.Since(copyStart))

		if dErr := conn.SetDeadline(time.Now().Add(legacyUploadFinalReadDeadline)); dErr != nil {
			return errors.Wrap(dErr, "set final response deadline")
		}

		readOKStart := time.Now()
		err = binnapi.ReadMessage(conn, &resp)
		if err != nil {
			slog.Debug("legacy upload: read final response failed",
				"node_id", node.ID, "attempt", attempt,
				"duration", time.Since(readOKStart),
				"total_attempt_duration", time.Since(attemptStart), "error", err)

			return errors.WithMessage(err, "failed to read upload response")
		}
		slog.Debug("legacy upload: final response received",
			"node_id", node.ID, "attempt", attempt,
			"duration", time.Since(readOKStart),
			"status_code", resp.Code, "info", resp.Info)

		if resp.Code != binnapi.StatusCodeOK {
			return errors.Errorf("upload failed with status code %d: %s", resp.Code, resp.Info)
		}

		slog.Debug("legacy upload: attempt succeeded",
			"node_id", node.ID, "attempt", attempt,
			"total_attempt_duration", time.Since(attemptStart))

		return nil
	})
	if err != nil {
		slog.Debug("legacy upload: all attempts failed",
			"node_id", node.ID, "file_path", filePath,
			"attempts", attempt, "error", err)

		return errors.WithMessagef(
			err,
			"failed to upload stream after %d attempts",
			filesRetryCount,
		)
	}

	slog.Debug("legacy upload: success",
		"node_id", node.ID, "file_path", filePath, "size", size, "attempts", attempt)

	return nil
}

// Remove removes a file or directory.
func (s *FileBINNService) Remove(ctx context.Context, node *domain.Node, path string, recursive bool) error {
	cfg, err := s.configMaker.MakeWithMode(ctx, node, binnapi.ModeFiles)
	if err != nil {
		return errors.WithMessage(err, "failed to make config")
	}

	pool, err := s.getPool(node.ID, cfg)
	if err != nil {
		return errors.WithMessage(err, "failed to get pool")
	}

	var resp binnapi.BaseResponseMessage

	err = Retry(filesRetryCount, filesRetryDelay, func() error {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return errors.WithMessage(err, "failed to acquire connection from pool")
		}
		defer func() {
			err = conn.Close()
			if err != nil {
				slog.Warn("failed to close connection", "error", err)
			}
		}()

		err = binnapi.WriteMessage(conn, &binnapi.RemoveRequestMessage{
			Path:      path,
			Recursive: recursive,
		})
		if err != nil {
			return errors.WithMessage(err, "failed to write remove request")
		}

		err = binnapi.ReadMessage(conn, &resp)
		if err != nil {
			return errors.WithMessage(err, "failed to read remove response")
		}

		return nil
	})
	if err != nil {
		return errors.WithMessagef(
			err,
			"failed to remove file after %d attempts",
			filesRetryCount,
		)
	}

	if resp.Code != binnapi.StatusCodeOK {
		return errors.Errorf("remove failed with status code %d: %s", resp.Code, resp.Info)
	}

	return nil
}

// GetFileInfo gets detailed information about a file.
func (s *FileBINNService) GetFileInfo(ctx context.Context, node *domain.Node, path string) (*FileDetails, error) {
	cfg, err := s.configMaker.MakeWithMode(ctx, node, binnapi.ModeFiles)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to make config")
	}

	pool, err := s.getPool(node.ID, cfg)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get pool")
	}

	var resp binnapi.BaseResponseMessage

	err = Retry(filesRetryCount, filesRetryDelay, func() error {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return errors.WithMessage(err, "failed to acquire connection from pool")
		}
		defer func() {
			err = conn.Close()
			if err != nil {
				slog.Warn("failed to close connection", "error", err)
			}
		}()

		err = binnapi.WriteMessage(conn, &binnapi.FileInfoRequestMessage{
			Path: path,
		})
		if err != nil {
			return errors.WithMessage(err, "failed to write file info request")
		}

		err = binnapi.ReadMessage(conn, &resp)
		if err != nil {
			return errors.WithMessage(err, "failed to read file info response")
		}

		return nil
	})
	if err != nil {
		return nil, errors.WithMessagef(
			err,
			"failed to get file info after %d attempts",
			filesRetryCount,
		)
	}

	if resp.Code != binnapi.StatusCodeOK {
		return nil, errors.Errorf("file info failed with status code %d: %s", resp.Code, resp.Info)
	}

	binnapiFileDetails, err := binnapi.CreateFileDetailsResponseMessage(resp.Data)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse file details")
	}

	return &FileDetails{
		Name:             binnapiFileDetails.Name,
		Mime:             binnapiFileDetails.Mime,
		Size:             binnapiFileDetails.Size,
		ModificationTime: binnapiFileDetails.ModificationTime,
		AccessTime:       binnapiFileDetails.AccessTime,
		CreateTime:       binnapiFileDetails.CreateTime,
		Perm:             binnapiFileDetails.Perm,
		Type:             FileType(binnapiFileDetails.Type),
	}, nil
}

// Chmod changes file permissions.
func (s *FileBINNService) Chmod(ctx context.Context, node *domain.Node, path string, perm uint32) error {
	cfg, err := s.configMaker.MakeWithMode(ctx, node, binnapi.ModeFiles)
	if err != nil {
		return errors.WithMessage(err, "failed to make config")
	}

	pool, err := s.getPool(node.ID, cfg)
	if err != nil {
		return errors.WithMessage(err, "failed to get pool")
	}

	var resp binnapi.BaseResponseMessage

	err = Retry(filesRetryCount, filesRetryDelay, func() error {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return errors.WithMessage(err, "failed to acquire connection from pool")
		}
		defer func() {
			err = conn.Close()
			if err != nil {
				slog.Warn("failed to close connection", "error", err)
			}
		}()

		err = binnapi.WriteMessage(conn, &binnapi.ChmodMessage{
			Path: path,
			Perm: perm,
		})
		if err != nil {
			return errors.WithMessage(err, "failed to write chmod request")
		}

		err = binnapi.ReadMessage(conn, &resp)
		if err != nil {
			return errors.WithMessage(err, "failed to read chmod response")
		}

		return nil
	})
	if err != nil {
		return errors.WithMessagef(
			err,
			"failed to change file permissions after %d attempts",
			filesRetryCount,
		)
	}

	if resp.Code != binnapi.StatusCodeOK {
		return errors.Errorf("chmod failed with status code %d: %s", resp.Code, resp.Info)
	}

	return nil
}

func (s *FileBINNService) getPool(nodeID uint, cfg config) (*Pool, error) {
	s.mu.RLock()
	pool, exists := s.pools[nodeID]
	s.mu.RUnlock()

	if exists {
		return pool, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check existence to avoid race condition
	pool, exists = s.pools[nodeID]
	if exists {
		return pool, nil
	}

	pool, err := NewPool(cfg)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create pool")
	}

	s.pools[nodeID] = pool

	return pool, nil
}

// idleDeadlineWriter resets the conn deadline before every Write. This
// implements an "idle timeout" for io.Copy — slow but progressing streams stay
// alive forever, while genuine stalls (no data flowing) trip the deadline.
type idleDeadlineWriter struct {
	conn        net.Conn
	idleTimeout time.Duration
}

func (w *idleDeadlineWriter) Write(b []byte) (int, error) {
	if w.idleTimeout > 0 {
		_ = w.conn.SetDeadline(time.Now().Add(w.idleTimeout))
	}

	return w.conn.Write(b)
}
